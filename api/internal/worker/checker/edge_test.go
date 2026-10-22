package checker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestDNSRejectsPrivateResolution(t *testing.T) {
	// localhost resolves to a loopback address -> rejected when private disallowed.
	res := checker.NewDNS(false).Check(context.Background(), "localhost", 5)
	assert.Equal(t, "down", res.Status)
	assert.Contains(t, res.ErrorMessage, "private or internal")
}

func TestHTTPRetriesOn5xx(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	res := checker.NewHTTP(0, "", true).Check(context.Background(), srv.URL, 5)
	assert.Equal(t, "down", res.Status)
	assert.GreaterOrEqual(t, calls, 2, "5xx should be retried once")
}
