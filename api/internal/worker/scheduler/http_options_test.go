package scheduler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/store"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFingerprintTracksHTTPOptions(t *testing.T) {
	no := false
	base := store.Monitor{ID: 1, Type: "http", Url: "https://example.com", IntervalSeconds: 60}
	changes := map[string]func(*store.Monitor){
		"method":            func(m *store.Monitor) { m.Method = "HEAD" },
		"headers":           func(m *store.Monitor) { m.Headers = `{"X-A":"1"}` },
		"body":              func(m *store.Monitor) { m.Body = "x" },
		"auth_type":         func(m *store.Monitor) { m.AuthType = "bearer" },
		"auth_username":     func(m *store.Monitor) { m.AuthUsername = "u" },
		"auth_secret":       func(m *store.Monitor) { m.AuthSecret = "s" },
		"tls_verify":        func(m *store.Monitor) { m.TlsVerify = &no },
		"ca_pem":            func(m *store.Monitor) { m.CaPem = "pem" },
		"follow_redirects":  func(m *store.Monitor) { m.FollowRedirects = &no },
		"accepted_statuses": func(m *store.Monitor) { m.AcceptedStatuses = "301" },
		"keyword_mode":      func(m *store.Monitor) { m.KeywordMode = "not_contains" },
	}
	for name, change := range changes {
		changed := base
		change(&changed)
		assert.NotEqual(t, fingerprint(base), fingerprint(changed), name)
	}

	// NULL and an explicit true are the same setting.
	yes := true
	explicit := base
	explicit.TlsVerify, explicit.FollowRedirects = &yes, &yes
	assert.Equal(t, fingerprint(base), fingerprint(explicit))

	// Quoting keeps a "|" in one field from matching a shift between fields.
	a, b := base, base
	a.Body, a.AuthUsername = "x|y", ""
	b.Body, b.AuthUsername = "x", "y"
	assert.NotEqual(t, fingerprint(a), fingerprint(b))
}

func TestHTTPOptionsFromMonitor(t *testing.T) {
	no := false
	expected := int64(301)
	opts, err := httpOptions(store.Monitor{
		ExpectedStatus: &expected, KeywordCheck: "error", KeywordMode: "not_contains",
		Method: "POST", Headers: `{"X-A":"1"}`, Body: "b",
		AuthType: "basic", AuthUsername: "u", AuthSecret: "p",
		TlsVerify: &no, CaPem: "pem", FollowRedirects: &no, AcceptedStatuses: "200-299,404",
	})
	require.NoError(t, err)
	assert.Equal(t, 301, opts.ExpectedStatus)
	assert.True(t, opts.KeywordAbsent)
	assert.Equal(t, map[string]string{"X-A": "1"}, opts.Headers)
	assert.Equal(t, []monitorvalidation.StatusRange{{Lo: 200, Hi: 299}, {Lo: 404, Hi: 404}}, opts.AcceptedStatuses)
	assert.True(t, opts.SkipTLSVerify)
	assert.True(t, opts.NoFollowRedirects)
	assert.Equal(t, "p", opts.AuthSecret)

	def, err := httpOptions(store.Monitor{})
	require.NoError(t, err)
	assert.False(t, def.SkipTLSVerify, "NULL tls_verify means verify")
	assert.False(t, def.NoFollowRedirects, "NULL follow_redirects means follow")
	assert.False(t, def.KeywordAbsent)

	_, err = httpOptions(store.Monitor{Headers: "not json"})
	assert.ErrorContains(t, err, "stored headers")
	_, err = httpOptions(store.Monitor{AcceptedStatuses: "abc"})
	assert.Error(t, err)
}

func TestCheckUsesMonitorHTTPOptions(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodHead || r.Header.Get("X-Api-Key") != "k1" {
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	db := testutil.NewTestDB(t)
	q := store.New(db)
	ctx := context.Background()
	s := New(db, checkresult.New(db, incident.NewDetector(q), alerter.NewDispatcher(q, config.Config{})), true)
	s.SetChecker("http", checker.NewHTTP(0, "", true))

	mon, err := q.CreateMonitor(ctx, store.CreateMonitorParams{
		Name: "api", Url: srv.URL, Type: "http", IntervalSeconds: 60, TimeoutSeconds: 5,
		DegradedThresholdMs: 5000, DownThresholdMs: 10000, IsActive: true, Source: "internal",
		Method: "HEAD", Headers: `{"X-Api-Key":"k1"}`,
	})
	require.NoError(t, err)
	s.check(ctx, mon)
	assert.Equal(t, "up", latestStatus(t, q, mon.ID))

	// A stored option that does not parse records a down result instead of
	// running the check with the wrong settings.
	mon.Headers = "not json"
	s.check(ctx, mon)
	cr, err := q.LatestCheckResult(ctx, mon.ID)
	require.NoError(t, err)
	assert.Equal(t, "down", cr.Status)
	assert.Contains(t, cr.ErrorMessage, "stored headers")
}
