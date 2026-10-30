package db_test

import (
	"context"
	"database/sql"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSchemaVersionOfAFreshDatabaseIsTheLatest(t *testing.T) {
	latest, err := db.LatestVersion()
	require.NoError(t, err)
	assert.GreaterOrEqual(t, latest, uint(14))

	conn, err := db.Open(t.TempDir() + "/v.db")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	version, dirty, err := db.SchemaVersion(context.Background(), conn)
	require.NoError(t, err)
	assert.Equal(t, latest, version)
	assert.False(t, dirty)
}

func TestSchemaVersionNeedsTheMigrationsTable(t *testing.T) {
	conn, err := sql.Open("sqlite", t.TempDir()+"/plain.db")
	require.NoError(t, err)
	t.Cleanup(func() { assert.NoError(t, conn.Close()) })
	_, _, err = db.SchemaVersion(context.Background(), conn)
	require.Error(t, err)
}
