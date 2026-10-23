package handlers_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/memetics19/pulse/api/internal/config"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupBody(username, sqlitePath string) []byte {
	b, _ := json.Marshal(map[string]string{
		"username": username, "password": "s3cret-pass", "site_name": username, "sqlite_path": sqlitePath,
	})
	return b
}

// fireSetups runs one POST /api/setup per handler, all released at once, and
// returns the status codes in handler order.
func fireSetups(handlerFor func(i int) http.HandlerFunc, n int, sqlitePath string) []int {
	codes := make([]int, n)
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodPost, "/api/setup",
				bytes.NewReader(setupBody(fmt.Sprintf("admin%d", i), sqlitePath)))
			rec := httptest.NewRecorder()
			<-start
			handlerFor(i)(rec, req)
			codes[i] = rec.Code
		}()
	}
	close(start)
	wg.Wait()
	return codes
}

// onlyWinner asserts exactly one 201 and 409 for every other request, and
// returns the winner's index.
func onlyWinner(t *testing.T, codes []int) int {
	t.Helper()
	winner, created := -1, 0
	for i, c := range codes {
		switch c {
		case http.StatusCreated:
			winner = i
			created++
		case http.StatusConflict:
		default:
			t.Fatalf("request %d got %d, want 201 or 409", i, c)
		}
	}
	require.Equal(t, 1, created, "exactly one setup must succeed: %v", codes)
	return winner
}

func countUsers(t *testing.T, conn generated.DBTX) int64 {
	t.Helper()
	n, err := generated.New(conn).CountUsers(context.Background())
	require.NoError(t, err)
	return n
}

// One server, many concurrent setup requests: the mutex lets one through and
// every other request sees the configured app.
func TestSetupConcurrentRequestsCreateOneAdmin(t *testing.T) {
	const n = 32
	dataDir := t.TempDir()
	a := app.New()
	h := handlers.NewSetup(a, dataDir, config.Config{})

	codes := fireSetups(func(int) http.HandlerFunc { return h.Complete }, n, filepath.Join(dataDir, "pulse.db"))
	onlyWinner(t, codes)

	require.True(t, a.Configured())
	t.Cleanup(func() { _ = a.DB().Close() })
	assert.Equal(t, int64(1), countUsers(t, a.DB()))
}

// Separate servers (separate in-process locks, separate data dirs) sharing one
// database file stand in for separate processes: only the database guard
// stands between them.
func TestSetupConcurrentServersShareOneDatabase(t *testing.T) {
	const n = 8
	path := filepath.Join(t.TempDir(), "pulse.db")
	conn, err := db.Open(path) // migrate once up front
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })

	apps := make([]*app.App, n)
	dirs := make([]string, n)
	hs := make([]*handlers.Setup, n)
	for i := range n {
		apps[i], dirs[i] = app.New(), t.TempDir()
		hs[i] = handlers.NewSetup(apps[i], dirs[i], config.Config{})
	}

	codes := fireSetups(func(i int) http.HandlerFunc { return hs[i].Complete }, n, path)
	winner := onlyWinner(t, codes)

	assert.Equal(t, int64(1), countUsers(t, conn))
	for i := range n {
		_, statErr := os.Stat(filepath.Join(dirs[i], "pulse.json"))
		if i == winner {
			require.True(t, apps[i].Configured())
			t.Cleanup(func() { _ = apps[i].DB().Close() })
			assert.NoError(t, statErr, "winner saves its bootstrap config")
			continue
		}
		assert.False(t, apps[i].Configured(), "loser %d must stay unconfigured", i)
		assert.ErrorIs(t, statErr, os.ErrNotExist, "loser %d must not save a bootstrap config", i)
	}
}

// A failed setup rolls the admin back, so a retry can still complete.
func TestSetupFailureLeavesNothingBehind(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pulse.db")
	notADir := filepath.Join(t.TempDir(), "file")
	require.NoError(t, os.WriteFile(notADir, []byte("x"), 0o600))

	broken := handlers.NewSetup(app.New(), filepath.Join(notADir, "data"), config.Config{})
	rec := httptest.NewRecorder()
	broken.Complete(rec, httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(setupBody("admin", path))))
	require.Equal(t, http.StatusInternalServerError, rec.Code)

	conn, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	assert.Equal(t, int64(0), countUsers(t, conn))

	a := app.New()
	rec = httptest.NewRecorder()
	handlers.NewSetup(a, t.TempDir(), config.Config{}).Complete(rec,
		httptest.NewRequest(http.MethodPost, "/api/setup", bytes.NewReader(setupBody("admin", path))))
	require.Equal(t, http.StatusCreated, rec.Code)
	t.Cleanup(func() { _ = a.DB().Close() })
	assert.Equal(t, int64(1), countUsers(t, conn))
}
