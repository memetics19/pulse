package checker

import (
	"context"
	"time"
)

// Result is the outcome of a single monitor check.
type Result struct {
	Status         string // "up", "down", "degraded"
	ResponseTimeMs int64  // milliseconds; 0 if not applicable
	StatusCode     int    // HTTP status code; reused for SSL days-until-expiry
	ErrorMessage   string // empty on success
	CheckedAt      time.Time
}

// Checker runs a single check against a target and returns a Result.
type Checker interface {
	Check(ctx context.Context, target string, timeoutSec int64) Result
}
