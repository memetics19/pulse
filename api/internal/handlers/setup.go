package handlers

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

// Setup handles the first-run wizard endpoints.
type Setup struct {
	app     *app.App
	dataDir string
	secure  bool
}

// NewSetup constructs a Setup handler bound to the given app and data directory.
// secure controls whether session cookies are marked Secure.
func NewSetup(a *app.App, dataDir string, secure bool) *Setup {
	return &Setup{app: a, dataDir: dataDir, secure: secure}
}

// State reports whether the instance has been configured yet and provides the
// default SQLite path so the frontend can pre-fill the form.
func (s *Setup) State(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":          s.app.Configured(),
		"default_sqlite_path": s.dataDir + "/pulse.db",
	})
}

type setupReq struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	SiteName   string `json:"site_name"`
	LogoURL    string `json:"logo_url"`
	SQLitePath string `json:"sqlite_path"`
}

// Complete performs the one-time setup: opens (or creates) the database,
// creates the admin user, seeds the theme config, persists the bootstrap
// config, and issues a session cookie for the new admin.
func (s *Setup) Complete(w http.ResponseWriter, r *http.Request) {
	if s.app.Configured() {
		http.Error(w, "already configured", http.StatusForbidden)
		return
	}

	var req setupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username and password (min 8) required", http.StatusBadRequest)
		return
	}

	path := req.SQLitePath
	if path == "" {
		path = s.dataDir + "/pulse.db"
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		http.Error(w, "could not create data directory: "+err.Error(), http.StatusBadRequest)
		return
	}
	conn, err := db.Open(path)
	if err != nil {
		http.Error(w, "could not open database: "+err.Error(), http.StatusBadRequest)
		return
	}

	q := generated.New(conn)

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}

	u, err := q.CreateUser(r.Context(), generated.CreateUserParams{
		Username:     req.Username,
		PasswordHash: hash,
		Name:         req.Name,
		Email:        req.Email,
	})
	if err != nil {
		http.Error(w, "could not create admin", http.StatusInternalServerError)
		return
	}

	cfg, _ := json.Marshal(map[string]string{
		"site_name": req.SiteName,
		"logo_url":  req.LogoURL,
	})
	// UpsertTheme is an INSERT … ON CONFLICT DO UPDATE, so it is safe to call
	// on a fresh database that has no existing row yet.
	_, _ = q.UpsertTheme(r.Context(), generated.UpsertThemeParams{
		Preset:     "default-light",
		CustomCss:  "",
		ConfigJson: string(cfg),
	})

	if err := bootstrap.Save(s.dataDir, bootstrap.Config{Configured: true, SQLitePath: path}); err != nil {
		http.Error(w, "could not persist config", http.StatusInternalServerError)
		return
	}
	s.app.SetDB(conn)

	if token, err := auth.NewSessionToken(); err == nil {
		_ = q.CreateSession(r.Context(), generated.CreateSessionParams{
			Token:     token,
			UserID:    u.ID,
			ExpiresAt: time.Now().Add(auth.SessionDuration),
		})
		auth.SetSessionCookie(w, token, s.secure)
	}

	w.WriteHeader(http.StatusCreated)
}
