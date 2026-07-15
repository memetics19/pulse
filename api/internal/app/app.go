package app

import (
	"database/sql"
	"sync"
	"sync/atomic"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
)

// App holds the (initially absent) database so the server can run in setup
// mode and flip to configured once setup completes.
type App struct {
	mu sync.RWMutex
	db *sql.DB
	q  *generated.Queries

	// workerBeat is the unix-nano timestamp of the worker's last successful
	// reconcile. /healthz uses it to report whether monitoring is actually
	// alive, so a silently-dead worker fails the health check instead of the
	// process looking healthy while nothing is being monitored.
	workerBeat atomic.Int64
}

func New() *App { return &App{} }

// MarkWorkerAlive records that the worker reconcile loop just ran successfully.
func (a *App) MarkWorkerAlive() { a.workerBeat.Store(time.Now().UnixNano()) }

// WorkerHealthy reports whether the worker beat within maxAge. It is false
// before the worker's first beat (unconfigured or not yet started); callers
// that must tolerate the setup phase should check Configured() first.
func (a *App) WorkerHealthy(maxAge time.Duration) bool {
	last := a.workerBeat.Load()
	return last != 0 && time.Since(time.Unix(0, last)) < maxAge
}

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
