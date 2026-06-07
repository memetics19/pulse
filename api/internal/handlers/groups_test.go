package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupsCRUD(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewGroups(q)

	body, _ := json.Marshal(map[string]any{
		"name":          "Core API",
		"display_order": 1,
		"description":   "Core services",
	})
	req := httptest.NewRequest(http.MethodPost, "/api/groups", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Create(rr, req)
	require.Equal(t, http.StatusCreated, rr.Code)

	var group generated.MonitorGroup
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&group))
	assert.Equal(t, "Core API", group.Name)

	req2 := httptest.NewRequest(http.MethodGet, "/api/groups", nil)
	rr2 := httptest.NewRecorder()
	h.List(rr2, req2)
	require.Equal(t, http.StatusOK, rr2.Code)
	var groups []generated.MonitorGroup
	require.NoError(t, json.NewDecoder(rr2.Body).Decode(&groups))
	assert.Len(t, groups, 1)
}
