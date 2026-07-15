package testutil

import (
	"context"
	"database/sql"
	"errors"
	"sync/atomic"

	"github.com/memetics19/pulse/api/internal/generated"
)

// errFault is returned by a tripped FaultDBTX call.
var errFault = errors.New("testutil: injected database failure")

// faultDBTX wraps a real *sql.DB and starts returning errors after failAfter
// successful calls. It lets tests exercise the "second/third DB call failed"
// error branches in handlers that make several queries, which a simply-closed
// DB (which fails the first call) cannot reach.
type faultDBTX struct {
	real      *sql.DB
	calls     atomic.Int32
	failAfter int32
}

// FailAfter returns a generated.DBTX that delegates the first n calls to real
// and fails every call after that. Use n=0 to fail immediately.
func FailAfter(real *sql.DB, n int) generated.DBTX {
	return &faultDBTX{real: real, failAfter: int32(n)}
}

func (f *faultDBTX) trip() bool { return f.calls.Add(1) > f.failAfter }

func (f *faultDBTX) ExecContext(ctx context.Context, q string, a ...interface{}) (sql.Result, error) {
	if f.trip() {
		return nil, errFault
	}
	return f.real.ExecContext(ctx, q, a...)
}

func (f *faultDBTX) QueryContext(ctx context.Context, q string, a ...interface{}) (*sql.Rows, error) {
	if f.trip() {
		return nil, errFault
	}
	return f.real.QueryContext(ctx, q, a...)
}

func (f *faultDBTX) QueryRowContext(ctx context.Context, q string, a ...interface{}) *sql.Row {
	if f.trip() {
		// A deliberately invalid query yields a *sql.Row whose Scan errors,
		// which is how the caller observes the injected failure.
		return f.real.QueryRowContext(ctx, "SELECT this is not valid sql")
	}
	return f.real.QueryRowContext(ctx, q, a...)
}

func (f *faultDBTX) PrepareContext(ctx context.Context, q string) (*sql.Stmt, error) {
	if f.trip() {
		return nil, errFault
	}
	return f.real.PrepareContext(ctx, q)
}
