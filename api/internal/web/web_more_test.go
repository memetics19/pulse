package web

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

// closedQ returns queries over a closed DB so calls error.
func closedWebQ(t *testing.T) *generated.Queries {
	t.Helper()
	db := testutil.NewTestDB(t)
	db.Close()
	return generated.New(db)
}

func TestPublicGetScopedPage(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	shown, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "Shown"})
	hidden, _ := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "Hidden"})
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

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "acme.com"
	rec := httptest.NewRecorder()
	NewPublic(q).Get(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("scoped page = %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "ShownMon") || strings.Contains(body, "HiddenMon") {
		t.Fatal("scoped page should show only ShownMon")
	}
}

func TestPublicUnpublishedPage404(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	q.CreateStatusPage(context.Background(), generated.CreateStatusPageParams{Domain: "hidden.com", Title: "H", Published: 0})
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Host = "hidden.com"
	rec := httptest.NewRecorder()
	NewPublic(q).Get(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unpublished page = %d, want 404", rec.Code)
	}
}

func TestPublicHandlersDBError(t *testing.T) {
	p := NewPublic(closedWebQ(t))
	for name, h := range map[string]http.HandlerFunc{
		"Get":        p.Get,
		"Feed":       p.Feed,
		"StatusJSON": p.StatusJSON,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code == http.StatusOK {
			t.Errorf("%s on closed DB should not be 200", name)
		}
	}
}

func TestPublicSnapshotErrorStages(t *testing.T) {
	// Snapshot issues several queries; failing partway makes buildVM/Feed/JSON
	// return their error path.
	for _, k := range []int{1, 2, 3} {
		db := testutil.NewTestDB(t)
		p := NewPublic(generated.New(testutil.FailAfter(db, k)))
		for _, h := range []http.HandlerFunc{p.Get, p.Feed, p.StatusJSON} {
			rec := httptest.NewRecorder()
			h(rec, httptest.NewRequest(http.MethodGet, "/", nil))
			if rec.Code == http.StatusOK {
				t.Errorf("fail-after-%d handler returned 200, expected error", k)
			}
		}
	}
}
