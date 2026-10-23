package handlers_test

import (
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
)

// Bad input is the client's fault and gets a 400 that names the problem.
// Server-side failures are covered in setup_test.go.
func TestSetupCompleteErrors(t *testing.T) {
	h := handlers.NewSetup(app.New(), t.TempDir(), config.Config{SetupToken: config.SetupTokenOff})

	assert.Equal(t, http.StatusBadRequest, do(h.Complete, "POST", "/x", "not json").Code)
	assert.Equal(t, http.StatusBadRequest,
		do(h.Complete, "POST", "/x", map[string]any{"username": "a", "password": "short"}).Code)
	assert.Equal(t, http.StatusBadRequest,
		do(h.Complete, "POST", "/x", map[string]any{"password": "s3cret-pass"}).Code)
}
