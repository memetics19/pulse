package importcmd_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/memetics19/pulse/cli/importcmd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const backupJSON = `{
  "monitorList": [
    {"id":1,"name":"API","url":"https://api.example.com","type":"http","interval":60,"maxretries":3,"keyword":"ok","tags":[]},
    {"id":2,"name":"Push","url":"","type":"push","interval":30,"maxretries":1,"keyword":"","tags":[]},
    {"id":3,"name":"Unknown","url":"","type":"grpc","interval":60,"maxretries":1,"keyword":"","tags":[]}
  ],
  "notificationList":[]
}`

func TestRunImport_CreatesGroupThenMonitors(t *testing.T) {
	var groupCreated atomic.Bool
	var monitorsCreated atomic.Int32

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer secret", r.Header.Get("Authorization"))

		switch r.URL.Path {
		case "/api/groups":
			require.Equal(t, http.MethodPost, r.Method)
			var body map[string]interface{}
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			assert.Equal(t, "Imported", body["name"])
			groupCreated.Store(true)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": float64(10), "name": "Imported", "display_order": float64(0), "description": "", "created_at": "2026-06-04T00:00:00Z"})

		case "/api/monitors":
			require.Equal(t, http.MethodPost, r.Method)
			monitorsCreated.Add(1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]interface{}{"id": float64(monitorsCreated.Load()), "name": "x"})

		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	// Write backup to a temp file
	tmpDir := t.TempDir()
	backupFile := filepath.Join(tmpDir, "backup.json")
	require.NoError(t, os.WriteFile(backupFile, []byte(backupJSON), 0o600))

	// Execute the command
	cmd := importcmd.NewImportCmd()
	cmd.SetArgs([]string{"uptime-kuma", "--file", backupFile, "--server", srv.URL, "--token", "secret"})
	err := cmd.Execute()
	require.NoError(t, err)

	assert.True(t, groupCreated.Load(), "group should have been created")
	// 2 monitors imported: "API" (http) and "Push" (push→http); "Unknown" (grpc) skipped
	assert.Equal(t, int32(2), monitorsCreated.Load(), "exactly 2 monitors should be POSTed")
}

func TestRunImport_MissingFile(t *testing.T) {
	cmd := importcmd.NewImportCmd()
	cmd.SetArgs([]string{"uptime-kuma", "--file", "/nonexistent/backup.json", "--server", "http://localhost:8080", "--token", "x"})
	err := cmd.Execute()
	assert.Error(t, err)
}
