package monitorvalidation

import (
	"net"
	"strings"

	"github.com/memetics19/pulse/api/internal/netguard"
)

type Input struct {
	URL             string
	Type            string
	IntervalSeconds int64
}

var validTypes = map[string]struct{}{
	"http": {}, "https": {}, "tcp": {}, "ping": {},
	"dns": {}, "ssl": {}, "infra": {}, "push": {},
}

// Validate returns a human-readable validation error, or an empty string when
// the monitor input is valid.
func Validate(in Input, allowPrivate bool) string {
	if in.IntervalSeconds < 1 {
		return "interval_seconds must be at least 1"
	}
	if _, ok := validTypes[in.Type]; !ok {
		return "invalid type"
	}
	if in.Type == "push" {
		return ""
	}
	if in.URL == "" {
		return "url is required"
	}
	if in.Type == "tcp" {
		if strings.Contains(in.URL, "://") {
			return "tcp target must be host:port"
		}
		host, port, err := net.SplitHostPort(in.URL)
		if err != nil || host == "" || port == "" {
			return "tcp target must be host:port"
		}
		if err := netguard.ValidateHostPort(in.URL, allowPrivate); err != nil {
			return err.Error()
		}
	}
	if in.Type == "ssl" {
		host, port, err := net.SplitHostPort(in.URL)
		if err != nil || host == "" || port == "" {
			return "ssl target must be host:port"
		}
		if err := netguard.ValidateHostPort(in.URL, allowPrivate); err != nil {
			return err.Error()
		}
	}
	// DNS monitors only resolve the name, but a name that resolves to an
	// internal address would make Pulse an oracle for internal DNS, so the same
	// host guard applies (the DNS checker re-checks the answer at run time).
	if in.Type == "ping" || in.Type == "dns" {
		if err := netguard.ValidateHost(in.URL, allowPrivate); err != nil {
			return err.Error()
		}
	}
	if in.Type == "http" || in.Type == "https" {
		if err := netguard.ValidateURL(in.URL, allowPrivate); err != nil {
			return err.Error()
		}
	}
	return ""
}
