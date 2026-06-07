package pulseclient_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/cli/internal/pulseclient"
	"github.com/memetics19/pulse/cli/internal/uptimekuma"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateGroup_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/groups", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "Imported", body["name"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":            float64(42),
			"name":          "Imported",
			"display_order": float64(0),
			"description":   "",
			"created_at":    "2026-06-04T00:00:00Z",
		})
	}))
	defer srv.Close()

	c := pulseclient.New(srv.URL, "test-token")
	groupID, err := c.CreateGroup("Imported")
	require.NoError(t, err)
	assert.Equal(t, int64(42), groupID)
}

func TestCreateGroup_Non201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
	}))
	defer srv.Close()

	c := pulseclient.New(srv.URL, "wrong-token")
	_, err := c.CreateGroup("Imported")
	assert.ErrorContains(t, err, "unexpected status 401")
}

func TestCreateMonitor_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, http.MethodPost, r.Method)
		assert.Equal(t, "/api/monitors", r.URL.Path)
		assert.Equal(t, "Bearer test-token", r.Header.Get("Authorization"))

		var body map[string]interface{}
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		assert.Equal(t, "My API", body["name"])
		assert.Equal(t, "http", body["type"])
		assert.Equal(t, float64(60), body["interval_seconds"])
		assert.Equal(t, float64(42), body["group_id"])
		assert.Equal(t, "uptime-kuma", body["source"])

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{"id": float64(7), "name": "My API"})
	}))
	defer srv.Close()

	c := pulseclient.New(srv.URL, "test-token")
	m := uptimekuma.PulseMonitor{
		Name:            "My API",
		URL:             "https://api.example.com",
		Type:            "http",
		IntervalSeconds: 60,
		KeywordCheck:    "ok",
	}
	err := c.CreateMonitor(m, 42)
	assert.NoError(t, err)
}

func TestCreateMonitor_Non201(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer srv.Close()

	c := pulseclient.New(srv.URL, "test-token")
	err := c.CreateMonitor(uptimekuma.PulseMonitor{Name: "X", Type: "http"}, 1)
	assert.ErrorContains(t, err, "unexpected status 400")
}
