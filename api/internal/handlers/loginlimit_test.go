package handlers

import (
	"net"
	"net/http"
	"testing"
)

func mustCIDR(s string) *net.IPNet { _, n, _ := net.ParseCIDR(s); return n }

func TestClientIP(t *testing.T) {
	trusted := []*net.IPNet{mustCIDR("10.0.0.0/8")}

	newReq := func(remote, xff string) *http.Request {
		r := httptest_New(remote)
		if xff != "" {
			r.Header.Set("X-Forwarded-For", xff)
		}
		return r
	}

	// Untrusted peer: XFF ignored, use RemoteAddr.
	if got := clientIP(newReq("203.0.113.9:5555", "1.2.3.4"), trusted); got != "203.0.113.9" {
		t.Errorf("untrusted peer: got %q, want 203.0.113.9", got)
	}
	// Trusted proxy: use rightmost non-proxy XFF entry.
	if got := clientIP(newReq("10.0.0.5:80", "8.8.8.8, 10.0.0.9"), trusted); got != "8.8.8.8" {
		t.Errorf("trusted proxy: got %q, want 8.8.8.8", got)
	}
	// No trusted proxies configured: always RemoteAddr, XFF ignored.
	if got := clientIP(newReq("10.0.0.5:80", "8.8.8.8"), nil); got != "10.0.0.5" {
		t.Errorf("no trusted: got %q, want 10.0.0.5", got)
	}
}

func httptest_New(remote string) *http.Request {
	r, _ := http.NewRequest(http.MethodPost, "/api/auth/login", nil)
	r.RemoteAddr = remote
	return r
}
