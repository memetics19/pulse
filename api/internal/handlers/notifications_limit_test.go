package handlers

import (
	"testing"
	"time"
)

func TestTestLimiter(t *testing.T) {
	now := time.Unix(1000, 0)
	l := newTestLimiter(10 * time.Second)
	l.now = func() time.Time { return now }

	if w := l.take(1); w != 0 {
		t.Fatalf("first test waits %v", w)
	}
	if w := l.take(2); w != 0 {
		t.Fatalf("another notification waits %v", w)
	}
	now = now.Add(4 * time.Second)
	if w := l.take(1); w != 6*time.Second {
		t.Fatalf("second test after 4s waits %v, want 6s", w)
	}
	now = now.Add(6 * time.Second)
	if w := l.take(1); w != 0 {
		t.Fatalf("test after 10s waits %v", w)
	}

	// Old entries are swept once the map grows.
	for id := int64(10); id < 1012; id++ {
		l.take(id)
	}
	now = now.Add(time.Minute)
	l.take(5000)
	if len(l.last) != 1 {
		t.Fatalf("after the sweep %d entries are left, want 1", len(l.last))
	}
}
