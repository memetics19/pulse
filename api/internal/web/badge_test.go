package web

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type badgeFixture struct {
	q              *generated.Queries
	api, db, other generated.Monitor
}

// newBadgeFixture has two groups. "shown" holds API and DB and is the only
// group on the published page at status.example.com. "hidden" holds Other.
// An unpublished page lives at draft.example.com.
func newBadgeFixture(t *testing.T) badgeFixture {
	t.Helper()
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()

	shown, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "shown"})
	require.NoError(t, err)
	hidden, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "hidden"})
	require.NoError(t, err)
	mk := func(name, url string, gid int64) generated.Monitor {
		m, err := q.CreateMonitor(ctx, generated.CreateMonitorParams{
			Name: name, Url: url, Type: "http", IntervalSeconds: 60, TimeoutSeconds: 10,
			DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true,
			GroupID: &gid, Source: "internal",
		})
		require.NoError(t, err)
		return m
	}
	f := badgeFixture{
		q:     q,
		api:   mk("API", "https://192.0.2.10/secret-target", shown.ID),
		db:    mk("DB", "tcp://192.0.2.11:5432", shown.ID),
		other: mk("Other", "https://192.0.2.12/hidden", hidden.ID),
	}

	page, err := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "status.example.com", Title: "Acme", Published: 1})
	require.NoError(t, err)
	require.NoError(t, q.AddPageGroup(ctx, generated.AddPageGroupParams{StatusPageID: page.ID, GroupID: shown.ID}))
	draft, err := q.CreateStatusPage(ctx, generated.CreateStatusPageParams{Domain: "draft.example.com", Title: "Draft", Published: 0})
	require.NoError(t, err)
	require.NoError(t, q.AddPageGroup(ctx, generated.AddPageGroupParams{StatusPageID: draft.ID, GroupID: shown.ID}))
	return f
}

func (f badgeFixture) check(t *testing.T, m generated.Monitor, status string, ago time.Duration) {
	t.Helper()
	_, err := f.q.InsertCheckResult(context.Background(), generated.InsertCheckResultParams{
		MonitorID: m.ID, CheckedAt: time.Now().Add(-ago), Status: status,
		ErrorMessage: "dial tcp 192.0.2.10:443: connection refused",
	})
	require.NoError(t, err)
}

func (f badgeFixture) get(host, target string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = host
	rec := httptest.NewRecorder()
	p := NewPublic(f.q)
	if strings.HasPrefix(target, "/badge/uptime.svg") {
		p.UptimeBadge(rec, req)
	} else {
		p.StatusBadge(rec, req)
	}
	return rec
}

// badgeText returns the title of a badge, which reads "label: message",
// and fails when the body is not well-formed XML.
func badgeText(t *testing.T, rec *httptest.ResponseRecorder) string {
	t.Helper()
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	dec := xml.NewDecoder(strings.NewReader(rec.Body.String()))
	var title string
	inTitle := false
	for {
		tok, err := dec.Token()
		if errors.Is(err, io.EOF) {
			break
		}
		require.NoError(t, err, "badge must be well-formed XML")
		switch v := tok.(type) {
		case xml.StartElement:
			inTitle = v.Name.Local == "title"
		case xml.CharData:
			if inTitle {
				title += string(v)
			}
		case xml.EndElement:
			inTitle = false
		}
	}
	return title
}

func TestStatusBadgeShowsPageOverallStatus(t *testing.T) {
	f := newBadgeFixture(t)
	const host = "status.example.com"

	assert.Equal(t, "status: operational", badgeText(t, f.get(host, "/badge/status.svg")))

	// A hidden monitor going down does not move the page's badge.
	f.check(t, f.other, "down", time.Minute)
	assert.Equal(t, "status: operational", badgeText(t, f.get(host, "/badge/status.svg")))

	f.check(t, f.db, "degraded", time.Minute)
	assert.Equal(t, "status: degraded", badgeText(t, f.get(host, "/badge/status.svg")))

	f.check(t, f.api, "down", time.Minute)
	assert.Equal(t, "status: outage", badgeText(t, f.get(host, "/badge/status.svg")))

	// The default page shows every group.
	assert.Equal(t, "status: outage", badgeText(t, f.get("unknown.example.com", "/badge/status.svg")))
}

func TestStatusBadgeForOneMonitor(t *testing.T) {
	f := newBadgeFixture(t)
	const host = "status.example.com"

	assert.Equal(t, "API: no data", badgeText(t, f.get(host, "/badge/status.svg?monitor=API")))
	f.check(t, f.api, "up", time.Minute)
	f.check(t, f.db, "down", time.Minute)
	assert.Equal(t, "API: up", badgeText(t, f.get(host, "/badge/status.svg?monitor=api")), "names match ignoring case")
	assert.Equal(t, "DB: down", badgeText(t, f.get(host, "/badge/status.svg?monitor="+strconv.FormatInt(f.db.ID, 10))))
	f.check(t, f.db, "degraded", 0)
	assert.Equal(t, "DB: degraded", badgeText(t, f.get(host, "/badge/status.svg?monitor=DB")))

	rec := f.get(host, "/badge/status.svg?monitor=API")
	assert.Equal(t, "image/svg+xml; charset=utf-8", rec.Header().Get("Content-Type"))
	assert.Equal(t, "public, max-age=60", rec.Header().Get("Cache-Control"))
	assert.Equal(t, "default-src 'none'", rec.Header().Get("Content-Security-Policy"))
	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
}

func TestBadgesStayScopedToThePage(t *testing.T) {
	f := newBadgeFixture(t)
	f.check(t, f.api, "down", time.Minute)
	f.check(t, f.other, "down", time.Minute)
	otherID := strconv.FormatInt(f.other.ID, 10)

	for _, target := range []string{
		"/badge/status.svg?monitor=Other",
		"/badge/status.svg?monitor=" + otherID,
		"/badge/uptime.svg?monitor=Other",
		"/badge/uptime.svg?monitor=" + otherID,
		"/badge/status.svg?monitor=nope",
	} {
		rec := f.get("status.example.com", target)
		assert.Equal(t, http.StatusNotFound, rec.Code, "%s must not reach a monitor off the page", target)
		assert.NotContains(t, rec.Body.String(), "Other")
	}

	// An unpublished page answers 404 for every badge.
	for _, target := range []string{"/badge/status.svg", "/badge/uptime.svg", "/badge/status.svg?monitor=API"} {
		assert.Equal(t, http.StatusNotFound, f.get("draft.example.com", target).Code, target)
	}

	// No badge carries a target or a raw error.
	for _, target := range []string{
		"/badge/status.svg", "/badge/status.svg?monitor=API",
		"/badge/uptime.svg", "/badge/uptime.svg?monitor=API",
	} {
		body := f.get("status.example.com", target).Body.String()
		for _, leak := range []string{"192.0.2.", "secret-target", "connection refused", "Other"} {
			assert.NotContains(t, body, leak, "%s leaks %q", target, leak)
		}
	}
}

func TestUptimeBadge(t *testing.T) {
	f := newBadgeFixture(t)
	const host = "status.example.com"

	assert.Equal(t, "uptime 30d: no data", badgeText(t, f.get(host, "/badge/uptime.svg")))

	for _, st := range []string{"up", "up", "up", "down"} {
		f.check(t, f.api, st, time.Hour)
	}
	f.check(t, f.db, "up", time.Hour)
	f.check(t, f.db, "down", 48*time.Hour) // outside 24h, inside 7d
	f.check(t, f.other, "down", time.Hour) // hidden, never counted

	assert.Equal(t, "API uptime 30d: 75%", badgeText(t, f.get(host, "/badge/uptime.svg?monitor=API")))
	assert.Equal(t, "DB uptime 24h: 100%", badgeText(t, f.get(host, "/badge/uptime.svg?monitor=DB&range=24h")))
	assert.Equal(t, "DB uptime 7d: 50%", badgeText(t, f.get(host, "/badge/uptime.svg?monitor=DB&range=7d")))
	// The page badge is the mean of its monitors: (75 + 100) / 2.
	assert.Equal(t, "uptime 24h: 87.5%", badgeText(t, f.get(host, "/badge/uptime.svg?range=24h")))

	rec := f.get(host, "/badge/uptime.svg?range=1y")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestBadgeEscapesMonitorNames(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	ctx := context.Background()
	g, err := q.CreateGroup(ctx, generated.CreateGroupParams{Name: "g"})
	require.NoError(t, err)
	name := `<script>alert("x")</script> & co`
	_, err = q.CreateMonitor(ctx, generated.CreateMonitorParams{
		Name: name, Url: "https://example.com", Type: "http", IntervalSeconds: 60, TimeoutSeconds: 10,
		DegradedThresholdMs: 500, DownThresholdMs: 2000, IsActive: true, GroupID: &g.ID, Source: "internal",
	})
	require.NoError(t, err)

	f := badgeFixture{q: q}
	rec := f.get("example.com", "/badge/status.svg?monitor="+strings.ReplaceAll(strings.ReplaceAll(name, " ", "+"), "&", "%26"))
	assert.Equal(t, name+": no data", badgeText(t, rec))
	assert.NotContains(t, rec.Body.String(), "<script>")
}

func TestBadgeHelpers(t *testing.T) {
	assert.Equal(t, "100%", formatPct(100))
	assert.Equal(t, "99.5%", formatPct(99.5))
	assert.Equal(t, "99.95%", formatPct(99.951))
	assert.Equal(t, "0%", formatPct(0))

	rec := httptest.NewRecorder()
	writeBadge(rec, strings.Repeat("a", 60), "up", badgeGreen)
	assert.Contains(t, rec.Body.String(), strings.Repeat("a", maxBadgeLabel-1)+"…")
	assert.NotContains(t, rec.Body.String(), strings.Repeat("a", maxBadgeLabel))
}
