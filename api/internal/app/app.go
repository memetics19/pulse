package app

import (
	"database/sql"
	"sync"

	"github.com/memetics19/pulse/api/internal/generated"
)

// App holds the (initially absent) database so the server can run in setup
// mode and flip to configured once setup completes.
type App struct {
	mu sync.RWMutex
	db *sql.DB
	q  *generated.Queries
}

func New() *App { return &App{} }

// SetDB installs an open, migrated database and marks the app configured.
func (a *App) SetDB(db *sql.DB) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.db = db
	a.q = generated.New(db)
}

func (a *App) Configured() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db != nil
}

// Queries returns the query handle, or nil when unconfigured.
func (a *App) Queries() *generated.Queries {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.q
}

// DB returns the raw handle (nil when unconfigured) for the worker.
func (a *App) DB() *sql.DB {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.db
}
