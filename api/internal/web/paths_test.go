package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestPublicRendersNoDataAndResolvedIncident(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	g, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "G"})
	// A monitor with no check results -> "no data" bars / "—" uptime.
	q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "Fresh", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000,
		IsActive: true, GroupID: &g.ID, Source: "internal",
	})
	// A resolved incident -> PastIncidents branch.
	rca := "root cause"
	inc, _ := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "Past outage", Severity: "minor", AffectedMonitorIds: "[]",
		StartedAt: time.Now().Add(-time.Hour), Source: "internal",
	})
	q.UpdateIncidentStatus(ctx, generated.UpdateIncidentStatusParams{Status: "resolved", Rca: &rca, ID: inc.ID})

	rec := httptest.NewRecorder()
	NewPublic(q).Get(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("render = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Fresh") {
		t.Fatal("expected the no-data monitor to render")
	}
}
