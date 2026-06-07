package app

import (
	"context"
	"database/sql"

	"github.com/memetics19/pulse/api/internal/generated"
)

// LiveDBTX returns a generated.DBTX that forwards every call to the App's
// current database. Safe to build before the DB exists; only invoked for
// DB-backed routes, which the setup gate blocks until the app is configured.
func LiveDBTX(a *App) generated.DBTX { return liveDBTX{a} }

type liveDBTX struct{ a *App }

func (x liveDBTX) ExecContext(ctx context.Context, q string, args ...interface{}) (sql.Result, error) {
	return x.a.DB().ExecContext(ctx, q, args...)
}

func (x liveDBTX) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	return x.a.DB().PrepareContext(ctx, q)
}

func (x liveDBTX) QueryContext(ctx context.Context, q string, args ...interface{}) (*sql.Rows, error) {
	return x.a.DB().QueryContext(ctx, q, args...)
}

func (x liveDBTX) QueryRowContext(ctx context.Context, q string, args ...interface{}) *sql.Row {
	return x.a.DB().QueryRowContext(ctx, q, args...)
}
