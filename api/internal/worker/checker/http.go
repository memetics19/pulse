package checker

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/netguard"
)

const userAgent = "Pulse/1.0 (+https://github.com/memetics19/pulse)"

// Shared HTTP transports, built once and reused across every check. A transport
// pools and reuses connections; building a fresh one per check (as the code
// used to) both leaked idle connections/goroutines — a discarded transport does
// not close them on GC — and inflated latency by re-paying the TCP+TLS handshake
// every time. There is one transport per allowPrivate value because the netguard
// dial guard is baked into the dialer's Control hook; allowPrivate is
// process-wide config, so two transports cover every monitor.
var (
	sharedTransportPublic  = newSharedTransport(false)
	sharedTransportPrivate = newSharedTransport(true)
)

func newSharedTransport(allowPrivate bool) *http.Transport {
	return &http.Transport{
		DialContext: (&net.Dialer{
			Timeout: 30 * time.Second, // safety cap; the per-check context deadline is tighter
			Control: netguard.DialControl(allowPrivate),
		}).DialContext,
		MaxIdleConns:        100,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}
}

func sharedTransport(allowPrivate bool) *http.Transport {
	if allowPrivate {
		return sharedTransportPrivate
	}
	return sharedTransportPublic
}

type httpChecker struct {
	expectedStatus int
	keyword        string
	allowPrivate   bool
}

// NewHTTP creates a checker that GETs the target URL.
// expectedStatus 0 means "any 2xx is up". keyword "" means no body check.
// allowPrivate permits targets on private/internal networks (see netguard).
func NewHTTP(expectedStatus int, keyword string, allowPrivate bool) Checker {
	return &httpChecker{expectedStatus: expectedStatus, keyword: keyword, allowPrivate: allowPrivate}
}

// Check performs the request, retrying once on a transient failure (network
// error or 5xx) before concluding the target is down — this avoids flapping a
// healthy site to "down" on a single blip.
func (c *httpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second // never leave the request without a deadline
	}
	// One client per Check is fine — it's a thin wrapper; the shared transport
	// underneath is what pools connections. Per-monitor timeout is enforced by a
	// context deadline per attempt (client.Timeout can't live on a shared client).
	client := &http.Client{Transport: sharedTransport(c.allowPrivate)}

	var res Result
	for attempt := 0; attempt < 2; attempt++ {
		res = c.attempt(ctx, client, target, timeout)
		if res.Status == "up" {
			return res
		}
		// Only retry transient failures: network errors (StatusCode 0) or 5xx.
		// A 4xx or keyword mismatch is a real, stable failure — don't retry.
		if res.StatusCode != 0 && res.StatusCode < 500 {
			break
		}
		if attempt == 0 {
			select {
			case <-ctx.Done():
				return res
			case <-time.After(500 * time.Millisecond):
			}
		}
	}
	return res
}

func (c *httpChecker) attempt(ctx context.Context, client *http.Client, target string, timeout time.Duration) Result {
	// The per-attempt deadline replaces the old per-client Timeout, which can't
	// live on the shared client. Cancel fires after the body is read below.
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	req, err := http.NewRequestWithContext(actx, http.MethodGet, target, nil)
	if err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	req.Header.Set("User-Agent", userAgent)

	resp, err := client.Do(req)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	defer resp.Body.Close()

	// Status check: an explicit expected status requires an exact match;
	// otherwise any 2xx is considered up.
	ok := resp.StatusCode/100 == 2
	if c.expectedStatus != 0 {
		ok = resp.StatusCode == c.expectedStatus
	}
	if !ok {
		want := "2xx"
		if c.expectedStatus != 0 {
			want = fmt.Sprintf("%d", c.expectedStatus)
		}
		return Result{
			Status:         "down",
			ResponseTimeMs: elapsed,
			StatusCode:     resp.StatusCode,
			ErrorMessage:   fmt.Sprintf("expected status %s, got %d", want, resp.StatusCode),
			CheckedAt:      time.Now(),
		}
	}

	if c.keyword != "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if !strings.Contains(string(body), c.keyword) {
			return Result{
				Status:         "down",
				ResponseTimeMs: elapsed,
				StatusCode:     resp.StatusCode,
				ErrorMessage:   fmt.Sprintf("keyword %q not found in response body", c.keyword),
				CheckedAt:      time.Now(),
			}
		}
	}

	return Result{Status: "up", ResponseTimeMs: elapsed, StatusCode: resp.StatusCode, CheckedAt: time.Now()}
}
