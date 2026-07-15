package handlers_test

import (
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
)

func TestSetupCompleteErrors(t *testing.T) {
	valid := map[string]any{"username": "admin", "password": "s3cret-pass"}

	// bad body -> 400
	a := app.New()
	h := handlers.NewSetup(a, t.TempDir(), false)
	assert.Equal(t, http.StatusBadRequest, do(h.Complete, "POST", "/x", "not json").Code)

	// short password -> 400
	assert.Equal(t, http.StatusBadRequest,
		do(h.Complete, "POST", "/x", map[string]any{"username": "a", "password": "short"}).Code)

	// sqlite_path whose parent is a file -> MkdirAll fails -> 400
	dir := t.TempDir()
	file := filepath.Join(dir, "afile")
	os.WriteFile(file, []byte("x"), 0o600)
	body := map[string]any{"username": "admin", "password": "s3cret-pass", "sqlite_path": filepath.Join(file, "db.sqlite")}
	assert.Equal(t, http.StatusBadRequest, do(h.Complete, "POST", "/x", body).Code)

	// sqlite_path that is a directory -> db.Open fails -> 400
	body2 := map[string]any{"username": "admin", "password": "s3cret-pass", "sqlite_path": t.TempDir()}
	assert.Equal(t, http.StatusBadRequest, do(handlers.NewSetup(app.New(), t.TempDir(), false).Complete, "POST", "/x", body2).Code)

	_ = valid
}
