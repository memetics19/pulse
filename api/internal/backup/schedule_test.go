package backup_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/backup"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScheduleTakesDailySnapshotsAndKeepsTheNewest(t *testing.T) {
	data := t.TempDir()
	conn := newDB(t, filepath.Join(data, "pulse.db"), 3)
	configFile := filepath.Join(data, "pulse.json")
	require.NoError(t, os.WriteFile(configFile, []byte(`{}`), 0o600))

	dir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	// Files that are not automatic snapshots are never counted or removed.
	others := []string{"pulse-backup-20200101T000000Z.db", "pulse-auto-notatime.db", "notes.txt"}
	for _, name := range others {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), nil, 0o600))
	}

	s := backup.NewSchedule(conn, dir, 2, configFile)
	start := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)

	require.NoError(t, s.Run(ctx, start))
	first := "pulse-auto-20261030T030000Z.db"
	assert.ElementsMatch(t, append([]string{first, first + ".json"}, others...), names(t, dir))
	assert.Equal(t, 3, rows(t, filepath.Join(dir, first)))

	// Within a day of the newest snapshot, Run takes none.
	require.NoError(t, s.Run(ctx, start.Add(23*time.Hour)))
	assert.Len(t, names(t, dir), 5)

	require.NoError(t, s.Run(ctx, start.Add(24*time.Hour)))
	require.NoError(t, s.Run(ctx, start.Add(48*time.Hour)))
	assert.ElementsMatch(t, append([]string{
		"pulse-auto-20261031T030000Z.db", "pulse-auto-20261031T030000Z.db.json",
		"pulse-auto-20261101T030000Z.db", "pulse-auto-20261101T030000Z.db.json",
	}, others...), names(t, dir), "the oldest snapshot and its pulse.json are removed")
}

func TestScheduleWaitsAfterAFailure(t *testing.T) {
	conn := newDB(t, filepath.Join(t.TempDir(), "pulse.db"), 1)
	// A file in place of the directory makes every snapshot fail.
	dir := filepath.Join(t.TempDir(), "backups")
	require.NoError(t, os.WriteFile(dir, nil, 0o600))

	s := backup.NewSchedule(conn, dir, 0, "")
	start := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)
	require.Error(t, s.Run(ctx, start))
	require.NoError(t, s.Run(ctx, start.Add(time.Minute)), "no retry within the hour")

	require.NoError(t, os.Remove(dir))
	require.NoError(t, s.Run(ctx, start.Add(time.Hour)), "the next try after an hour")
	assert.Len(t, names(t, dir), 1, "keep is at least 1")
}

func TestScheduleSnapshotErrors(t *testing.T) {
	conn := newDB(t, filepath.Join(t.TempDir(), "pulse.db"), 1)
	dir := t.TempDir()
	s := backup.NewSchedule(conn, dir, 7, "")
	at := time.Date(2026, 10, 30, 3, 0, 0, 0, time.UTC)

	// A directory with the snapshot's name makes Snapshot refuse it.
	require.NoError(t, os.Mkdir(filepath.Join(dir, "pulse-auto-20261030T030000Z.db.json"), 0o750))
	require.ErrorIs(t, s.Run(ctx, at), backup.ErrExists)

	// A snapshot that cannot be removed fails the prune, after the new
	// snapshot is written.
	dir2 := t.TempDir()
	old := filepath.Join(dir2, "pulse-auto-20200101T000000Z.db")
	require.NoError(t, os.MkdirAll(filepath.Join(old+".json", "x"), 0o750))
	require.NoError(t, os.WriteFile(old, nil, 0o600))
	s2 := backup.NewSchedule(conn, dir2, 1, "")
	require.Error(t, s2.Run(ctx, at))
	assert.FileExists(t, filepath.Join(dir2, "pulse-auto-20261030T030000Z.db"))
}
