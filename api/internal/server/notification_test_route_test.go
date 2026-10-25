package server_test

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// POST /api/notifications/{id}/test needs notifications:write. With email off, an email
// notification's test succeeds without touching the network.
func TestNotificationTestRoute(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	n, err := q.CreateNotification(t.Context(), generated.CreateNotificationParams{
		Channel: "email", ConfigJson: `{"to":"ops@example.com"}`,
	})
	require.NoError(t, err)
	a := app.New()
	a.SetDB(db)
	h := server.New(a, t.TempDir(), config.Config{})

	post := func(path, key string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, path, nil)
		if key != "" {
			req.Header.Set("Authorization", "Bearer "+key)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	path := fmt.Sprintf("/api/notifications/%d/test", n.ID)

	assert.Equal(t, http.StatusUnauthorized, post(path, "").Code)
	assert.Equal(t, http.StatusForbidden, post(path, createServerAPIKey(t, q, "notifications:read")).Code)

	key := createServerAPIKey(t, q, "notifications:write")
	assert.Equal(t, http.StatusNotFound, post("/api/notifications/999999/test", key).Code)
	rec := post(path, key)
	assert.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.JSONEq(t, `{"ok":true}`, rec.Body.String())
	assert.Equal(t, http.StatusTooManyRequests, post(path, key).Code)
}
