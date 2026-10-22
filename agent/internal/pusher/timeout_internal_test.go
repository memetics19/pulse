package pusher

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/memetics19/pulse/agent/internal/collector"
	"github.com/memetics19/pulse/agent/internal/diagnostics"
	"github.com/stretchr/testify/require"
)

func slowServer(t *testing.T, delay time.Duration) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(delay)
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Metrics run on a ticker with no caller deadline, so the push must bound
// itself rather than risk a request that never returns.
func TestPush_BoundsItselfWhenCallerGivesNoDeadline(t *testing.T) {
	p := New(slowServer(t, 2*time.Second).URL, "t")
	p.metricsTimeout = 50 * time.Millisecond

	require.Error(t, p.Push(context.Background(), collector.Metrics{}))
}

// A bundle upload must use the budget its caller granted. The shared client
// timeout previously capped every request, silently truncating it.
func TestPushDiagnostics_UsesTheCallerBudgetNotTheMetricsTimeout(t *testing.T) {
	p := New(slowServer(t, 200*time.Millisecond).URL, "t")
	p.metricsTimeout = 20 * time.Millisecond // would fail the upload if applied

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	require.NoError(t, p.PushDiagnostics(ctx, diagnostics.Bundle{}))
}
