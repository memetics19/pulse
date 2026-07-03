package handlers

import (
	"net"
	"net/http"
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

// clientIP extracts the remote IP, ignoring proxy headers: Pulse cannot know
// whether a trustworthy proxy set them, and honoring them would let attackers
// spoof fresh rate-limit buckets.
func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
