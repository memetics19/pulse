package handlers

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
	// dbPath is the database file setup creates. Only the server decides it:
	// PULSE_DB, else pulse.db in PULSE_DATA_DIR, else the default. A request
	// can never name another file.
	dbPath string
	// token is the one-time setup token a request must carry. Empty means no
	// token is required, which only PULSE_SETUP_TOKEN=off allows.
	token  string
	secure bool

	// mu serializes Complete, so the configured check, the database open and
	// the admin insert run for one request at a time in this process.
	mu sync.Mutex
}

// setupDone is the reply to every setup attempt after the one that won.
const setupDone = "setup is already complete"

// setupFailed is the reply to every setup attempt that fails on the server.
// The detail goes to the server log, never to the client.
const setupFailed = "setup failed, see the server log"

// NewSetup constructs a Setup handler bound to the given app and data
// directory. The database file comes from cfg.SQLitePath, or pulse.db in
// dataDir when that is empty. cfg.SecureCookies controls whether session
// cookies are marked Secure.
//
// Until the app is configured, setup needs a token: cfg.SetupToken when set,
// else a random one generated here and logged once. cfg.SetupToken set to
// "off" turns the check off, for automated installs.
func NewSetup(a *app.App, dataDir string, cfg config.Config) *Setup {
	dbPath := cfg.SQLitePath
	if dbPath == "" {
		dbPath = filepath.Join(dataDir, config.DBFileName)
	}
	return &Setup{app: a, dataDir: dataDir, dbPath: dbPath, token: setupToken(a, cfg.SetupToken), secure: cfg.SecureCookies}
}

// setupToken returns the token setup requires, or "" for none. It logs how
// setup is guarded, and prints a generated token once, only while the app is
// not configured.
func setupToken(a *app.App, configured string) string {
	if a.Configured() {
		return ""
	}
	switch {
	case strings.EqualFold(configured, config.SetupTokenOff):
		log.Printf("setup token: off (PULSE_SETUP_TOKEN=off). Anyone who reaches /setup first can create the admin account")
		return ""
	case configured != "":
		log.Printf("setup token: open /setup and enter the value of PULSE_SETUP_TOKEN")
		return configured
	}
	token := rand.Text()
	log.Printf("setup token: open /setup and enter this token: %s", token)
	return token
}

// tokenMatches compares the submitted token with the expected one in constant
// time. Both are hashed first, so the comparison does not leak the length of
// the expected token either.
func tokenMatches(want, got string) bool {
	w := sha256.Sum256([]byte(want))
	g := sha256.Sum256([]byte(got))
	return subtle.ConstantTimeCompare(w[:], g[:]) == 1
}

// State reports whether the instance has been configured yet. Before setup it
// also reports the database file the server will create, for the wizard to
// show read-only, and whether setup needs a token. It never returns the token.
func (s *Setup) State(w http.ResponseWriter, r *http.Request) {
	if s.app.Configured() {
		writeJSON(w, http.StatusOK, map[string]any{"configured": true})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":           false,
		"sqlite_path":          s.dbPath,
		"setup_token_required": s.token != "",
	})
}

type setupReq struct {
	Name       string `json:"name"`
	Email      string `json:"email"`
	Username   string `json:"username"`
	Password   string `json:"password"`
	SiteName   string `json:"site_name"`
	LogoURL    string `json:"logo_url"`
	SetupToken string `json:"setup_token"`
	// SQLitePath is no longer accepted. It is decoded only to refuse it.
	SQLitePath string `json:"sqlite_path"`
}

// failSetup logs the detail of a failed setup and sends the client a generic 500.
func failSetup(w http.ResponseWriter, what string, err error) {
	log.Printf("setup: %s: %v", what, err)
	http.Error(w, setupFailed, http.StatusInternalServerError)
}

// Complete performs the one-time setup: opens (or creates) the database the
// server chose, creates the admin user, seeds the theme config, persists the
// bootstrap config, and issues a session cookie for the new admin. Only one
// call can ever succeed. Every other call gets 409 and changes nothing.
func (s *Setup) Complete(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.app.Configured() {
		http.Error(w, setupDone, http.StatusConflict)
		return
	}

	var req setupReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if s.token != "" && !tokenMatches(s.token, strings.TrimSpace(req.SetupToken)) {
		http.Error(w, "missing or wrong setup token, see the server log", http.StatusUnauthorized)
		return
	}
	if req.SQLitePath != "" {
		http.Error(w, "sqlite_path is not accepted: the server sets the database file from PULSE_DB or PULSE_DATA_DIR", http.StatusBadRequest)
		return
	}
	if req.Username == "" || len(req.Password) < 8 {
		http.Error(w, "username and password (min 8) required", http.StatusBadRequest)
		return
	}

	path := s.dbPath
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		failSetup(w, "create the data directory", err)
		return
	}
	conn, err := db.Open(path)
	if err != nil {
		failSetup(w, "open the database "+path, err)
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
		failSetup(w, "hash the password", err)
		return
	}

	// The admin, theme and session rows commit together, after the bootstrap
	// config is saved, so a failed setup leaves nothing behind and can be
	// retried. The transaction ignores client cancellation so it cannot roll
	// back after the config is written.
	ctx := context.WithoutCancel(r.Context())
	tx, err := conn.BeginTx(ctx, nil)
	if err != nil {
		failSetup(w, "begin the transaction", err)
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
		failSetup(w, "create the admin", err)
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
		failSetup(w, "create a session token", err)
		return
	}
	if err := q.CreateSession(ctx, generated.CreateSessionParams{
		Token:     token,
		UserID:    u.ID,
		ExpiresAt: time.Now().Add(auth.SessionDuration),
	}); err != nil {
		failSetup(w, "create the session", err)
		return
	}

	if err := bootstrap.Save(s.dataDir, bootstrap.Config{Configured: true, SQLitePath: path}); err != nil {
		failSetup(w, "save "+bootstrap.Path(s.dataDir), err)
		return
	}
	if err := tx.Commit(); err != nil {
		failSetup(w, "commit", err)
		return
	}
	s.app.SetDB(conn)
	installed = true
	auth.SetSessionCookie(w, token, s.secure)

	w.WriteHeader(http.StatusCreated)
}
