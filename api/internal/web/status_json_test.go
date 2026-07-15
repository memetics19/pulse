package web

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

// The public /api/status endpoint must never expose internal monitor
// configuration (target URL, thresholds, source, external ID). This is the
// regression guard for the data-boundary leak where the old handler serialized
// the raw monitor model to any caller.
func TestStatusJSONDoesNotLeakMonitorInternals(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	g, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "prod"})
	if err != nil {
		t.Fatal(err)
	}
	secret := "http://10.1.2.3:9000/internal-admin"
	if _, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "API", Url: secret, Type: "http", IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
		GroupID: &g.ID, Source: "internal", ExternalID: "ext-123",
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "anything.example" // default page (all groups)
	NewPublic(q).StatusJSON(rec, req)

	body := rec.Body.String()
	for _, leak := range []string{secret, "10.1.2.3", "ext-123", "external_id", "down_threshold", "\"url\""} {
		if strings.Contains(body, leak) {
			t.Fatalf("status JSON leaked %q; body=%s", leak, body)
		}
	}
	// The monitor name (public) should still be present.
	if !strings.Contains(body, "API") {
		t.Fatalf("expected monitor name in response; body=%s", body)
	}
}

// A scoped page must only show monitors from its selected groups.
func TestStatusJSONScopesToPageGroups(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	shown, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "shown"})
	hidden, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "hidden"})
	mk := func(name string, gid int64) {
		q.CreateMonitor(ctx, generated.CreateMonitorParams{
			Name: name, Url: "http://example.com", Type: "http", IntervalSeconds: 60,
			TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000,
			IsActive: true, GroupID: &gid, Source: "internal",
		})
	}
	mk("ShownMon", shown.ID)
	mk("HiddenMon", hidden.ID)

	page, _ := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "acme.com", Title: "Acme", Published: 1})
	q.AddPageGroup(ctx, generated.AddPageGroupParams{StatusPageID: page.ID, GroupID: shown.ID})

	rec := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/status", nil)
	req.Host = "acme.com"
	NewPublic(q).StatusJSON(rec, req)

	body := rec.Body.String()
	if !strings.Contains(body, "ShownMon") {
		t.Fatalf("expected ShownMon on its page; body=%s", body)
	}
	if strings.Contains(body, "HiddenMon") {
		t.Fatalf("HiddenMon from an unlisted group leaked onto the page; body=%s", body)
	}
}
