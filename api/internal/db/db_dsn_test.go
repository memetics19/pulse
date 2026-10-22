package db_test

import (
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
	"github.com/stretchr/testify/require"
)

func TestOpenMergesExistingFileURIOptions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "options.db")
	uri := (&url.URL{Scheme: "file", Path: path}).String() + "?mode=rwc&cache=private"

	conn, err := db.Open(uri)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}

func TestOpenEscapesSpecialCharactersInPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pulse ? #.db")

	conn, err := db.Open(path)
	require.NoError(t, err)
	require.NoError(t, conn.Close())
	_, err = os.Stat(path)
	require.NoError(t, err)
}

func TestOpenSupportsMemoryDatabase(t *testing.T) {
	conn, err := db.Open(":memory:")
	require.NoError(t, err)
	require.NoError(t, conn.Close())
}
