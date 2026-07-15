package handlers_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthReflectsWorker(t *testing.T) {
	db := testutil.NewTestDB(t)

	// Unconfigured: healthy (setup phase).
	a := app.New()
	assert.Equal(t, http.StatusOK, do(handlers.NewHealth(a).Get, "GET", "/healthz", nil).Code)

	// Configured + worker alive: healthy.
	a.SetDB(db)
	a.MarkWorkerAlive()
	assert.Equal(t, http.StatusOK, do(handlers.NewHealth(a).Get, "GET", "/healthz", nil).Code)

	// Configured + worker never beat: unhealthy.
	dead := app.New()
	dead.SetDB(db)
	assert.Equal(t, http.StatusServiceUnavailable, do(handlers.NewHealth(dead).Get, "GET", "/healthz", nil).Code)
}

func TestApiKeyCreateAndRevoke(t *testing.T) {
	q := newQ(t)
	h := handlers.NewAPIKeys(q)

	cr := do(h.Create, "POST", "/api/keys", map[string]any{"name": "ci", "scopes": []string{"monitors:read"}})
	require.Equal(t, http.StatusCreated, cr.Code)
	var created struct {
		ID int64 `json:"id"`
	}
	require.NoError(t, json.NewDecoder(cr.Body).Decode(&created))
	require.NotZero(t, created.ID)

	assert.Equal(t, http.StatusOK, do(h.List, "GET", "/api/keys", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Revoke(w, withChiID(r, "id", "bad"))
	}, "DELETE", "/x", nil).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Revoke(w, withChiID(r, "id", itoa(created.ID)))
	}, "DELETE", "/x", nil).Code)
}

func TestPagesUpdateDelete(t *testing.T) {
	q := newQ(t)
	ctx := context.Background()
	sp, err := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "acme.com", Title: "Acme", Published: 1})
	require.NoError(t, err)
	h := handlers.NewPages(q)
	id := itoa(sp.ID)

	assert.Equal(t, http.StatusOK, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", id))
	}, "PUT", "/x", map[string]any{"domain": "acme.io", "title": "Acme2", "published": true, "group_ids": []int64{}}).Code)
	assert.Equal(t, http.StatusNoContent, do(func(w http.ResponseWriter, r *http.Request) {
		h.Delete(w, withChiID(r, "id", id))
	}, "DELETE", "/x", nil).Code)
	assert.Equal(t, http.StatusBadRequest, do(func(w http.ResponseWriter, r *http.Request) {
		h.Update(w, withChiID(r, "id", "bad"))
	}, "PUT", "/x", map[string]any{}).Code)
}
