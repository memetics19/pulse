package server

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/go-chi/chi/v5"
	chimiddleware "github.com/go-chi/chi/v5/middleware"
	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/internal/middleware"
	"github.com/memetics19/pulse/api/internal/web"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
)

func New(a *app.App, dataDir string, cfg config.Config) http.Handler {
	q := generated.New(app.LiveDBTX(a))
	r := chi.NewRouter()

	r.Use(middleware.RequestLogger(os.Stdout))
	r.Use(chimiddleware.Recoverer)
	r.Use(middleware.MaxBody(1 << 20)) // 1 MiB request body cap
	r.Use(middleware.CORS(cfg.CORSOrigins))
	r.Use(middleware.SetupGate(a))

	// Setup wizard (always reachable, even unconfigured)
	setupH := handlers.NewSetup(a, dataDir, cfg.SecureCookies)
	r.Get("/api/setup/state", setupH.State)
	r.Post("/api/setup", setupH.Complete)

	// Public
	pub := web.NewPublic(q)
	r.Get("/", pub.Get)
	r.Get("/feed.xml", pub.Feed)
	r.Handle("/static/*", web.StaticHandler())

	r.Get("/healthz", handlers.Health)
	r.Get("/api/status", handlers.NewStatus(q).Get)
	r.Post("/api/ingest/metrics", handlers.NewIngest(q).PostMetrics)
	pushH := handlers.NewPush(q, &livePushRecorder{app: a, cfg: cfg})
	r.Get("/api/push/{token}", pushH.Heartbeat)
	r.Post("/api/push/{token}", pushH.Heartbeat)

	// Public read-only (status page client-side fetches)
	r.Get("/api/monitors/{monitorID}/checks", handlers.NewCheckResults(q).List)
	r.Get("/api/monitors/{monitorID}/checks/uptime", handlers.NewCheckResults(q).Uptime)
	r.Get("/api/incidents/{incidentID}/updates", handlers.NewIncidentUpdates(q).List)

	authH := handlers.NewAuth(q, cfg.SecureCookies)
	r.Post("/api/auth/login", authH.Login)
	r.Post("/api/auth/logout", authH.Logout)
	r.Get("/api/auth/status", authH.Status)

	// Admin (session-protected)
	// Key management is session-only — an API key must not manage API keys.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireSession(q))
		kh := handlers.NewAPIKeys(q)
		r.Get("/api/keys", kh.List)
		r.Post("/api/keys", kh.Create)
		r.Delete("/api/keys/{id}", kh.Revoke)

		sh := handlers.NewSettings(q)
		r.Get("/api/settings", sh.Get)
		r.Put("/api/settings", sh.Update)

		r.Post("/api/auth/2fa/setup", authH.TwoFASetup)
		r.Post("/api/auth/2fa/enable", authH.TwoFAEnable)
		r.Post("/api/auth/2fa/disable", authH.TwoFADisable)
	})

	// Admin/data API: a session cookie (full access) OR a scoped API key.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireSessionOrAPIKey(q))

		r.Route("/api/groups", func(r chi.Router) {
			h := handlers.NewGroups(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Put("/{id}", h.Update)
			r.Delete("/{id}", h.Delete)
		})

		r.Route("/api/monitors", func(r chi.Router) {
			h := handlers.NewMonitors(q, a.DB, cfg.AllowPrivateMonitors)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Get("/{id}", h.Get)
			r.Put("/{id}", h.Update)
			r.Delete("/{id}", h.Delete)
			r.Post("/{id}/push-token/rotate", h.RotatePushToken)
		})

		r.Route("/api/incidents", func(r chi.Router) {
			h := handlers.NewIncidents(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Put("/{id}/status", h.UpdateStatus)
			r.Delete("/{id}", h.Delete)
		})

		r.Post("/api/incidents/{incidentID}/updates", handlers.NewIncidentUpdates(q).Create)

		r.Route("/api/notifications", func(r chi.Router) {
			h := handlers.NewNotifications(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Put("/{id}", h.Update)
			r.Delete("/{id}", h.Delete)
		})

		r.Route("/api/agents", func(r chi.Router) {
			h := handlers.NewAgents(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Delete("/{id}", h.Delete)
		})

		r.Route("/api/agents/{agentID}/metrics", func(r chi.Router) {
			h := handlers.NewAgents(q)
			r.Get("/", h.GetMetrics)
		})

		r.Route("/api/theme", func(r chi.Router) {
			h := handlers.NewTheme(q)
			r.Get("/", h.Get)
			r.Put("/", h.Update)
		})

		r.Route("/api/maintenance", func(r chi.Router) {
			h := handlers.NewMaintenance(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Put("/{id}/status", h.UpdateStatus)
			r.Delete("/{id}", h.Delete)
		})

		r.Route("/api/pages", func(r chi.Router) {
			h := handlers.NewPages(q)
			r.Get("/", h.List)
			r.Post("/", h.Create)
			r.Put("/{id}", h.Update)
			r.Delete("/{id}", h.Delete)
		})

		r.Get("/api/overview", handlers.NewOverview(q).Get)
	})

	assets := web.StaticAssets()
	r.Handle("/admin", http.RedirectHandler("/admin/", http.StatusMovedPermanently))
	r.Handle("/admin/*", assets)
	r.Handle("/setup", http.RedirectHandler("/setup/", http.StatusMovedPermanently))
	r.Handle("/setup/*", assets)
	r.Handle("/_next/*", assets)
	r.Get("/favicon.ico", assets.ServeHTTP)

	return r
}

type livePushRecorder struct {
	app *app.App
	cfg config.Config
}

func (r *livePushRecorder) Record(ctx context.Context, monitor store.Monitor, in checkresult.Input) error {
	db := r.app.DB()
	if db == nil {
		return fmt.Errorf("database unavailable")
	}
	q := store.New(db)
	dispatcher := alerter.NewDispatcher(q, r.cfg.ResendAPIKey, r.cfg.SlackWebhookURL)
	detector := incident.NewDetector(q)
	return checkresult.New(db, detector, dispatcher).Record(ctx, monitor, in)
}
