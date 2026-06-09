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

// elapsedMs returns the milliseconds elapsed since start, floored to 1 for any
// completed measurement. A check that actually ran always took some nonzero
// time, so 0 is reserved for the "not applicable" case (see
// Result.ResponseTimeMs). Without the floor, a sub-millisecond response (e.g. a
// localhost target on a fast host) would truncate to 0 and be indistinguishable
// from "not measured".
func elapsedMs(start time.Time) int64 {
	if ms := time.Since(start).Milliseconds(); ms > 0 {
		return ms
	}
	return 1
}
