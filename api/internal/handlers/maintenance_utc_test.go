package handlers_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestMaintenanceCreateStoresOffsetInputAsUTC covers issue #48. An API client
// may send a time with an offset. The window must be stored in UTC so the
// worker's string comparison in SQLite sorts it correctly.
func TestMaintenanceCreateStoresOffsetInputAsUTC(t *testing.T) {
	db := testutil.NewTestDB(t)
	h := handlers.NewMaintenance(generated.New(db))

	rec := do(h.Create, "POST", "/x", map[string]any{
		"title":     "db upgrade",
		"starts_at": "2026-10-23T10:00:00+05:30",
		"ends_at":   "2026-10-23T12:00:00+05:30",
	})
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())

	var view struct {
		ID       int64     `json:"id"`
		StartsAt time.Time `json:"starts_at"`
		EndsAt   time.Time `json:"ends_at"`
	}
	require.NoError(t, json.NewDecoder(rec.Body).Decode(&view))
	assert.Equal(t, time.UTC, view.StartsAt.Location())
	assert.True(t, view.StartsAt.Equal(time.Date(2026, 10, 23, 4, 30, 0, 0, time.UTC)))

	var startsAt, endsAt string
	require.NoError(t, db.QueryRow(
		`SELECT CAST(starts_at AS TEXT), CAST(ends_at AS TEXT) FROM maintenance_windows WHERE id = ?`,
		view.ID).Scan(&startsAt, &endsAt))
	assert.Equal(t, "2026-10-23 04:30:00 +0000 UTC", startsAt)
	assert.Equal(t, "2026-10-23 06:30:00 +0000 UTC", endsAt)
}

func TestMaintenanceCreateRejectsEndNotAfterStart(t *testing.T) {
	h := handlers.NewMaintenance(newQ(t))
	start := time.Date(2026, 10, 23, 10, 0, 0, 0, time.UTC)

	cases := map[string]map[string]any{
		"equal": {
			"title": "t", "starts_at": start.Format(time.RFC3339), "ends_at": start.Format(time.RFC3339),
		},
		"before": {
			"title": "t", "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(-time.Hour).Format(time.RFC3339),
		},
		// Same instant written with two offsets is still not after.
		"equal across offsets": {
			"title": "t", "starts_at": "2026-10-23T10:00:00+05:30", "ends_at": "2026-10-23T04:30:00Z",
		},
		// start_now uses the current time, so an end in the past is rejected.
		"start now with past end": {
			"title": "t", "starts_at": start.Format(time.RFC3339), "ends_at": start.Add(time.Hour).Format(time.RFC3339),
			"start_now": true,
		},
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			rec := do(h.Create, "POST", "/x", body)
			assert.Equal(t, http.StatusBadRequest, rec.Code)
			assert.Contains(t, rec.Body.String(), "ends_at must be after starts_at")
		})
	}
}
