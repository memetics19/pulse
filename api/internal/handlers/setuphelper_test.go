package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/require"
)

func setupAdminExt(t *testing.T, h *handlers.Auth) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"username": "admin", "password": "s3cret-pass"})
	rec := httptest.NewRecorder()
	h.Setup(rec, httptest.NewRequest(http.MethodPost, "/api/auth/setup", bytes.NewReader(body)))
	require.Equal(t, http.StatusCreated, rec.Code)
}
