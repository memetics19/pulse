package handlers

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

// Setup handles the first-run wizard endpoints.
type Setup struct {
	app     *app.App
	dataDir string
	// dbPath is the database file the wizard creates unless the form names
	// another. dbFixed means PULSE_DB set it, and the form may not change it.
	dbPath  string
	dbFixed bool
	secure  bool

	// mu serializes Complete, so the configured check, the database open and
	// the admin insert run for one request at a time in this process.
	mu sync.Mutex
}

// setupDone is the reply to every setup attempt after the one that won.
const setupDone = "setup is already complete"

// NewSetup constructs a Setup handler bound to the given app and data
// directory. The database file comes from cfg.SQLitePath, or pulse.db in
// dataDir when that is empty. cfg.SecureCookies controls whether session
// cookies are marked Secure.
func NewSetup(a *app.App, dataDir string, cfg config.Config) *Setup {
	dbPath := cfg.SQLitePath
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, config.DBFileName)
	}
	return &Setup{app: a, dataDir: dataDir, dbPath: dbPath, dbFixed: cfg.SQLitePathSet, secure: cfg.SecureCookies}
}

// State reports whether the instance has been configured yet and provides the
// default SQLite path so the frontend can pre-fill the form. When PULSE_DB
// sets the path, sqlite_path_fixed tells the form not to offer another.
func (s *Setup) State(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":          s.app.Configured(),
		"default_sqlite_path": s.dbPath,
		"sqlite_path_fixed":   s.dbFixed,
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
// config, and issues a session cookie for the new admin. Only one call can
// ever succeed. Every other call gets 409 and changes nothing.
func (s *Setup) Complete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app.Configured() {
		http.Error(w, setupDone, http.StatusConflict)
		return
	}

	var req setupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username and password (min 8) required", http.StatusBadRequest)
		return
	}

	path := req.SQLitePath
	if path == "" {
		path = s.dbPath
	}
	if s.dbFixed && filepath.Clean(path) != filepath.Clean(s.dbPath) {
		http.Error(w, "the database file is set by PULSE_DB on the server: "+s.dbPath, http.StatusBadRequest)
		return
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
	installed := false
	defer func() {
		if !installed {
			_ = conn.Close()
		}
	}()

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		http.Error(w, "hash error", http.StatusInternalServerError)
		return
	}

	// The admin, theme and session rows commit together, after the bootstrap
	// config is saved, so a failed setup leaves nothing behind and can be
	// retried. The transaction ignores client cancellation so it cannot roll
	// back after the config is written.
	ctx := context.WithoutCancel(r.Context())
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	defer func() { _ = tx.Rollback() }()
	q := generated.New(tx)

	// CreateFirstUser inserts only into an empty users table, in one
	// statement. That holds across processes and for a database that already
	// has users.
	u, err := q.CreateFirstUser(ctx, generated.CreateFirstUserParams{
		Username:     req.Username,
		PasswordHash: hash,
		Name:         req.Name,
		Email:        req.Email,
	})
	if errors.Is(err, sql.ErrNoRows) {
		http.Error(w, setupDone, http.StatusConflict)
		return
	}
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
	_, _ = q.UpsertTheme(ctx, generated.UpsertThemeParams{
		Preset:     "default-light",
		CustomCss:  "",
		ConfigJson: string(cfg),
	})

	token, err := auth.NewSessionToken()
	if err != nil {
		http.Error(w, "session error", http.StatusInternalServerError)
		return
	}
	if err := q.CreateSession(ctx, generated.CreateSessionParams{
		Token:     token,
		UserID:    u.ID,
		ExpiresAt: time.Now().UTC().Add(auth.SessionDuration),
	}); err != nil {
		http.Error(w, "could not create session", http.StatusInternalServerError)
		return
	}

	if err := bootstrap.Save(s.dataDir, bootstrap.Config{Configured: true, SQLitePath: path}); err != nil {
		http.Error(w, "could not persist config", http.StatusInternalServerError)
		return
	}
	if err := tx.Commit(); err != nil {
		http.Error(w, "could not save setup", http.StatusInternalServerError)
		return
	}
	s.app.SetDB(conn)
	installed = true
	auth.SetSessionCookie(w, token, s.secure)

	w.WriteHeader(http.StatusCreated)
}
