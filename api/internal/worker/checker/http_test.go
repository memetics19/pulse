package checker_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/stretchr/testify/assert"
)

func TestHTTPChecker_Up(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	c := checker.NewHTTP(200, "")
	result := c.Check(context.Background(), srv.URL, 5)
	assert.Equal(t, "up", result.Status)
	assert.Equal(t, 200, result.StatusCode)
	assert.Greater(t, result.ResponseTimeMs, int64(0))
	assert.Empty(t, result.ErrorMessage)
}

func TestHTTPChecker_Down_WrongStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	c := checker.NewHTTP(200, "")
	result := c.Check(context.Background(), srv.URL, 5)
	assert.Equal(t, "down", result.Status)
	assert.Equal(t, 503, result.StatusCode)
}

func TestHTTPChecker_Down_Keyword(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("maintenance mode"))
	}))
	defer srv.Close()

	c := checker.NewHTTP(200, "healthy")
	result := c.Check(context.Background(), srv.URL, 5)
	assert.Equal(t, "down", result.Status)
	assert.Contains(t, result.ErrorMessage, "keyword")
}

func TestHTTPChecker_Down_Unreachable(t *testing.T) {
	c := checker.NewHTTP(200, "")
	result := c.Check(context.Background(), "http://localhost:19999", 1)
	assert.Equal(t, "down", result.Status)
	assert.NotEmpty(t, result.ErrorMessage)
}
