package checker

import (
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/monitorvalidation"
	"github.com/memetics19/pulse/api/internal/netguard"
)

const userAgent = "Pulse/1.0 (+https://github.com/memetics19/pulse)"

// maxRedirects matches the Go client's own default limit.
const maxRedirects = 10

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

// tlsTransports caches one transport per custom TLS setup (skip verify, or a
// private CA), for the same pooling reasons as the shared transports. Each is
// built by newSharedTransport, so the netguard dial guard applies to it too.
// The cache is reset when it grows past maxTLSTransports, which only happens
// when CA certificates are edited many times.
var tlsTransports = struct {
	sync.Mutex
	m map[string]*http.Transport
}{m: map[string]*http.Transport{}}

const maxTLSTransports = 64

func transportFor(allowPrivate, skipVerify bool, caPEM string) (*http.Transport, error) {
	if !skipVerify && caPEM == "" {
		return sharedTransport(allowPrivate), nil
	}
	sum := sha256.Sum256([]byte(caPEM))
	key := fmt.Sprintf("%t|%t|%x", allowPrivate, skipVerify, sum)

	tlsTransports.Lock()
	defer tlsTransports.Unlock()
	if t, ok := tlsTransports.m[key]; ok {
		return t, nil
	}
	cfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if skipVerify {
		cfg.InsecureSkipVerify = true // the monitor opted out of verification
	} else {
		// The private CA is trusted on top of the system roots, so a redirect
		// to a public site still verifies.
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM([]byte(caPEM)) {
			return nil, errors.New("ca_pem has no valid PEM certificate")
		}
		cfg.RootCAs = pool
	}
	if len(tlsTransports.m) >= maxTLSTransports {
		for _, t := range tlsTransports.m {
			t.CloseIdleConnections()
		}
		tlsTransports.m = map[string]*http.Transport{}
	}
	t := newSharedTransport(allowPrivate)
	t.TLSClientConfig = cfg
	tlsTransports.m[key] = t
	return t, nil
}

// HTTPOptions configures an HTTP check. Every zero value keeps the default
// behaviour: a GET with no body or auth, normal TLS verification, redirects
// followed, any 2xx accepted, and a keyword that must be present.
type HTTPOptions struct {
	// ExpectedStatus, when set, is a status that counts as up.
	ExpectedStatus int
	// AcceptedStatuses are further statuses that count as up.
	AcceptedStatuses []monitorvalidation.StatusRange
	// Keyword, when set, must appear in the body, or must not appear when
	// KeywordAbsent is set.
	Keyword       string
	KeywordAbsent bool
	// Method is GET, HEAD or POST. Empty means GET.
	Method  string
	Headers map[string]string
	// Body is sent with POST only.
	Body string
	// AuthType is "", "basic" or "bearer". AuthSecret is the basic password
	// or the bearer token.
	AuthType     string
	AuthUsername string
	AuthSecret   string
	// SkipTLSVerify turns certificate checks off. CAPEM trusts a private CA.
	SkipTLSVerify bool
	CAPEM         string
	// NoFollowRedirects reports the first response, so a 301 or 302 can be
	// the expected result.
	NoFollowRedirects bool
}

type httpChecker struct {
	opts         HTTPOptions
	allowPrivate bool
}

// NewHTTP creates a checker that GETs the target URL.
// expectedStatus 0 means "any 2xx is up". keyword "" means no body check.
// allowPrivate permits targets on private/internal networks (see netguard).
func NewHTTP(expectedStatus int, keyword string, allowPrivate bool) Checker {
	return NewHTTPWithOptions(HTTPOptions{ExpectedStatus: expectedStatus, Keyword: keyword}, allowPrivate)
}

// NewHTTPWithOptions creates a checker with the full set of HTTP options.
// allowPrivate permits targets on private/internal networks (see netguard).
func NewHTTPWithOptions(opts HTTPOptions, allowPrivate bool) Checker {
	return &httpChecker{opts: opts, allowPrivate: allowPrivate}
}

// Check performs the request, retrying once on a transient failure (network
// error or 5xx) before concluding the target is down — this avoids flapping a
// healthy site to "down" on a single blip.
func (c *httpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	timeout := time.Duration(timeoutSec) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second // never leave the request without a deadline
	}
	transport, err := transportFor(c.allowPrivate, c.opts.SkipTLSVerify, c.opts.CAPEM)
	if err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	// One client per Check is fine — it's a thin wrapper; the shared transport
	// underneath is what pools connections. Per-monitor timeout is enforced by a
	// context deadline per attempt (client.Timeout can't live on a shared client).
	client := &http.Client{Transport: transport, CheckRedirect: c.checkRedirect}

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

// checkRedirect stops at the first response when redirects are off. When they
// are on, every hop still dials through the netguard transport. The auth and
// the monitor's own headers may carry secrets, so they are only sent to the
// host and port the monitor targets. Go's own rule is looser: it keeps
// Authorization for a subdomain or another port on the same host.
func (c *httpChecker) checkRedirect(req *http.Request, via []*http.Request) error {
	if c.opts.NoFollowRedirects {
		return http.ErrUseLastResponse
	}
	if len(via) >= maxRedirects {
		return fmt.Errorf("stopped after %d redirects", maxRedirects)
	}
	if req.URL.Host != via[0].URL.Host {
		for name := range c.opts.Headers {
			req.Header.Del(name)
		}
		req.Header.Del("Authorization")
		req.Header.Set("User-Agent", userAgent)
	}
	return nil
}

func (c *httpChecker) newRequest(ctx context.Context, target string) (*http.Request, error) {
	method := c.opts.Method
	if method == "" {
		method = http.MethodGet
	}
	var body io.Reader
	if method == http.MethodPost && c.opts.Body != "" {
		body = strings.NewReader(c.opts.Body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	for name, value := range c.opts.Headers {
		req.Header.Set(name, value)
	}
	switch c.opts.AuthType {
	case monitorvalidation.AuthBasic:
		req.SetBasicAuth(c.opts.AuthUsername, c.opts.AuthSecret)
	case monitorvalidation.AuthBearer:
		req.Header.Set("Authorization", "Bearer "+c.opts.AuthSecret)
	}
	return req, nil
}

func (c *httpChecker) attempt(ctx context.Context, client *http.Client, target string, timeout time.Duration) Result {
	// The per-attempt deadline replaces the old per-client Timeout, which can't
	// live on the shared client. Cancel fires after the body is read below.
	actx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	start := time.Now()
	req, err := c.newRequest(actx, target)
	if err != nil {
		return Result{Status: "down", ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}

	resp, err := client.Do(req)
	elapsed := elapsedMs(start)
	if err != nil {
		return Result{Status: "down", ResponseTimeMs: elapsed, ErrorMessage: err.Error(), CheckedAt: time.Now()}
	}
	// The body is only read, so a failed close loses nothing.
	defer func() { _ = resp.Body.Close() }()

	if !c.statusOK(resp.StatusCode) {
		return Result{
			Status:         "down",
			ResponseTimeMs: elapsed,
			StatusCode:     resp.StatusCode,
			ErrorMessage:   fmt.Sprintf("expected status %s, got %d", c.wantStatus(), resp.StatusCode),
			CheckedAt:      time.Now(),
		}
	}

	if c.opts.Keyword != "" {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		found := strings.Contains(string(body), c.opts.Keyword)
		if found == c.opts.KeywordAbsent {
			msg := fmt.Sprintf("keyword %q not found in response body", c.opts.Keyword)
			if c.opts.KeywordAbsent {
				msg = fmt.Sprintf("keyword %q found in response body", c.opts.Keyword)
			}
			return Result{
				Status:         "down",
				ResponseTimeMs: elapsed,
				StatusCode:     resp.StatusCode,
				ErrorMessage:   msg,
				CheckedAt:      time.Now(),
			}
		}
	}

	return Result{Status: "up", ResponseTimeMs: elapsed, StatusCode: resp.StatusCode, CheckedAt: time.Now()}
}

// statusOK applies the status rules: with no expected status and no accepted
// statuses any 2xx is up. Otherwise the code must equal the expected status or
// fall in one of the accepted ranges.
func (c *httpChecker) statusOK(code int) bool {
	if c.opts.ExpectedStatus == 0 && len(c.opts.AcceptedStatuses) == 0 {
		return code/100 == 2
	}
	if c.opts.ExpectedStatus != 0 && code == c.opts.ExpectedStatus {
		return true
	}
	for _, r := range c.opts.AcceptedStatuses {
		if r.Contains(code) {
			return true
		}
	}
	return false
}

// wantStatus describes the accepted statuses for an error message.
func (c *httpChecker) wantStatus() string {
	var want []string
	if c.opts.ExpectedStatus != 0 {
		want = append(want, fmt.Sprintf("%d", c.opts.ExpectedStatus))
	}
	for _, r := range c.opts.AcceptedStatuses {
		want = append(want, r.String())
	}
	if len(want) == 0 {
		return "2xx"
	}
	return strings.Join(want, " or ")
}
