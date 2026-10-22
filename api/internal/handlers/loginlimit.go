package handlers

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

// loginLimiter is a per-IP token bucket for login attempts: burst of 10,
// refilling one attempt every 6 seconds. It exists to slow down online
// password guessing against the single admin account.
type loginLimiter struct {
	mu      sync.Mutex
	buckets map[string]*bucket
}

type bucket struct {
	tokens float64
	last   time.Time
}

const (
	loginBurst  = 10
	loginRefill = time.Second * 6 // one attempt per 6s
)

func newLoginLimiter() *loginLimiter {
	return &loginLimiter{buckets: map[string]*bucket{}}
}

// allow consumes one attempt for ip and reports whether it was available.
func (l *loginLimiter) allow(ip string) bool {
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()

	// Lazy sweep: drop buckets that have fully refilled, so the map does not
	// grow without bound across many client IPs.
	if len(l.buckets) > 1000 {
		for k, b := range l.buckets {
			if now.Sub(b.last) > loginRefill*loginBurst {
				delete(l.buckets, k)
			}
		}
	}

	b, ok := l.buckets[ip]
	if !ok {
		b = &bucket{tokens: loginBurst, last: now}
		l.buckets[ip] = b
	}
	b.tokens += now.Sub(b.last).Seconds() / loginRefill.Seconds()
	if b.tokens > loginBurst {
		b.tokens = loginBurst
	}
	b.last = now
	if b.tokens < 1 {
		return false
	}
	b.tokens--
	return true
}

// clientIP returns the address used to key the login rate limiter. Proxy
// headers are ignored by default (an attacker could spoof them to mint fresh
// buckets). Only when the immediate peer is a configured trusted proxy is
// X-Forwarded-For consulted: it is walked right-to-left and the first address
// that is not itself a trusted proxy is treated as the real client, so a single
// proxy IP doesn't collapse every visitor into one shared bucket.
func clientIP(r *http.Request, trusted []*net.IPNet) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	if len(trusted) == 0 || !ipInAny(host, trusted) {
		return host
	}
	parts := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	for i := len(parts) - 1; i >= 0; i-- {
		ip := strings.TrimSpace(parts[i])
		if ip != "" && !ipInAny(ip, trusted) {
			return ip
		}
	}
	return host
}

// ipInAny reports whether ipStr parses to an IP contained in one of nets.
func ipInAny(ipStr string, nets []*net.IPNet) bool {
	ip := net.ParseIP(ipStr)
	if ip == nil {
		return false
	}
	for _, n := range nets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}
