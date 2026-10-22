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

// seedFullStatus populates a DB with a group, monitors, a spread of check
// results (up/degraded/down), an incident with a markdown update, a maintenance
// window, and a theme carrying branding + a javascript: URL (which safeURL must
// drop). Rendering it exercises most of public.go.
func seedFullStatus(t *testing.T, q *generated.Queries) {
	t.Helper()
	ctx := context.Background()

	g, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "Core"})
	if err != nil {
		t.Fatal(err)
	}
	mon, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: "API", Url: "http://example.com", Type: "http", IntervalSeconds: 60,
		TimeoutSeconds: 10, DegradedThresholdMs: 500, DownThresholdMs: 2000,
		IsActive: true, GroupID: &g.ID, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for i, st := range []string{"up", "degraded", "down", "up"} {
		ms := int64(100 * (i + 1))
		if _, err := q.InsertCheckResult(ctx, generated.InsertCheckResultParams{
			MonitorID: mon.ID, CheckedAt: now.Add(-time.Duration(i) * time.Hour),
			Status: st, ResponseTimeMs: &ms,
		}); err != nil {
			t.Fatal(err)
		}
	}

	inc, err := q.CreateIncident(ctx, generated.CreateIncidentParams{
		Title: "Elevated errors", Severity: "major",
		AffectedMonitorIds: "[" + itoa(mon.ID) + "]", StartedAt: now, Source: "internal",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := q.CreateIncidentUpdate(ctx, generated.CreateIncidentUpdateParams{
		IncidentID: inc.ID, Status: "investigating",
		Message: "We are **investigating** the [issue](https://status.example.com).", Author: "admin",
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := q.CreateMaintenance(ctx, generated.CreateMaintenanceParams{
		Title: "DB upgrade", Status: "scheduled", AffectedMonitorIds: "[" + itoa(mon.ID) + "]",
		StartsAt: now.Add(time.Hour), EndsAt: now.Add(2 * time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	cfg := `{"site_name":"Acme Status","logo_url":"https://cdn.example.com/logo.png",` +
		`"favicon_url":"javascript:alert(1)","footer":{"brand_line":"Acme","links":[` +
		`{"label":"Home","url":"https://acme.com"},{"label":"Bad","url":"javascript:alert(1)"}]}}`
	if _, err := q.UpsertTheme(ctx, generated.UpsertThemeParams{
		Preset: "dark", CustomCss: "body{color:red} </style><script>x</script>", ConfigJson: cfg,
	}); err != nil {
		t.Fatal(err)
	}
}

func itoa(n int64) string {
	if n < 0 {
		return "-" + itoa(-n)
	}
	if n < 10 {
		return string(rune('0' + n))
	}
	return itoa(n/10) + string(rune('0'+n%10))
}

func TestPublicPageFullRenderAcrossRanges(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	seedFullStatus(t, q)
	h := NewPublic(q)

	for _, rng := range []string{"90d", "30d", "7d", "24h", "bogus"} {
		rec := httptest.NewRecorder()
		h.Get(rec, httptest.NewRequest(http.MethodGet, "/?range="+rng, nil))
		if rec.Code != http.StatusOK {
			t.Fatalf("range %s = %d, want 200", rng, rec.Code)
		}
		body := rec.Body.String()
		if !strings.Contains(body, "API") {
			t.Fatalf("range %s: expected monitor name in body", rng)
		}
		// safeURL must have dropped the javascript: favicon and footer link.
		if strings.Contains(body, "javascript:alert") {
			t.Fatalf("range %s: javascript: URL leaked into page", rng)
		}
		// sanitizeCSS must have stripped the </style> breakout.
		if strings.Contains(body, "</style><script>x") {
			t.Fatalf("range %s: unsanitized custom CSS breakout in page", rng)
		}
	}
}

func TestPublicFeed(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	seedFullStatus(t, q)
	rec := httptest.NewRecorder()
	NewPublic(q).Feed(rec, httptest.NewRequest(http.MethodGet, "/feed.xml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("feed = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "xml") {
		t.Fatalf("feed content-type = %q, want xml", ct)
	}
	if !strings.Contains(rec.Body.String(), "Elevated errors") {
		t.Fatal("feed should contain the incident title")
	}
}

func TestStaticHandlerServes(t *testing.T) {
	h := StaticHandler()
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/static/does-not-exist.css", nil))
	// 404 is fine — the point is the handler runs without panicking.
	if rec.Code != http.StatusNotFound && rec.Code != http.StatusOK {
		t.Fatalf("static handler unexpected code %d", rec.Code)
	}
}
