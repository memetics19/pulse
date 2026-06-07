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

func TestSettingsGet_DefaultRetention(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewSettings(q)

	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr := httptest.NewRecorder()
	h.Get(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]int
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 90, resp["retention_days"])
}

func TestSettingsUpdate_AndGet(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewSettings(q)

	// PUT retention_days = 30
	body, _ := json.Marshal(map[string]int{"retention_days": 30})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	var resp map[string]int
	require.NoError(t, json.NewDecoder(rr.Body).Decode(&resp))
	assert.Equal(t, 30, resp["retention_days"])

	// GET should now return 30
	req2 := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	rr2 := httptest.NewRecorder()
	h.Get(rr2, req2)

	require.Equal(t, http.StatusOK, rr2.Code)
	var resp2 map[string]int
	require.NoError(t, json.NewDecoder(rr2.Body).Decode(&resp2))
	assert.Equal(t, 30, resp2["retention_days"])
}

func TestSettingsUpdate_ZeroIsRejected(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h := handlers.NewSettings(q)

	body, _ := json.Marshal(map[string]int{"retention_days": 0})
	req := httptest.NewRequest(http.MethodPut, "/api/settings", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.Update(rr, req)

	assert.Equal(t, http.StatusBadRequest, rr.Code)
}
