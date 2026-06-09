package checker

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const userAgent = "Pulse/1.0 (+https://github.com/memetics19/pulse)"

type httpChecker struct {
	expectedStatus int
	keyword        string
}

// NewHTTP creates a checker that GETs the target URL.
// expectedStatus 0 means "any 2xx is up". keyword "" means no body check.
func NewHTTP(expectedStatus int, keyword string) Checker {
	return &httpChecker{expectedStatus: expectedStatus, keyword: keyword}
}

// Check performs the request, retrying once on a transient failure (network
// error or 5xx) before concluding the target is down — this avoids flapping a
// healthy site to "down" on a single blip.
func (c *httpChecker) Check(ctx context.Context, target string, timeoutSec int64) Result {
	client := &http.Client{Timeout: time.Duration(timeoutSec) * time.Second}

	var res Result
	for attempt := 0; attempt < 2; attempt++ {
		res = c.attempt(ctx, client, target)
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

func (c *httpChecker) attempt(ctx context.Context, client *http.Client, target string) Result {
	start := time.Now()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
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
