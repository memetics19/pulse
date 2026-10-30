package monitorvalidation

import (
	"crypto/x509"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// Limits on the HTTP monitor options. They keep one monitor row and one check
// request small.
const (
	MaxBodyBytes     = 64 << 10
	MaxCAPEMBytes    = 64 << 10
	MaxHeaders       = 32
	MaxHeaderValue   = 4 << 10
	MaxAuthField     = 4 << 10
	MaxStatusEntries = 32
)

// Keyword modes. An empty mode means KeywordContains.
const (
	KeywordContains    = "contains"
	KeywordNotContains = "not_contains"
)

// Auth types. An empty type means no auth.
const (
	AuthBasic  = "basic"
	AuthBearer = "bearer"
)

// HTTPOptions holds the optional per-monitor settings of an http or https
// monitor. Every zero value keeps the default behaviour.
type HTTPOptions struct {
	Method           string
	Headers          map[string]string
	Body             string
	AuthType         string
	AuthUsername     string
	AuthSecret       string
	CAPEM            string
	AcceptedStatuses []string
	KeywordMode      string
	Keyword          string
}

// reservedHeaders are set by the HTTP client itself or control the connection.
// Letting a monitor set them would break the request or route it somewhere
// the SSRF guard did not check. Authorization goes through the auth option so
// its secret stays write-only.
var reservedHeaders = map[string]struct{}{
	"Host": {}, "Content-Length": {}, "Transfer-Encoding": {},
	"Connection": {}, "Keep-Alive": {}, "Proxy-Connection": {},
	"Upgrade": {}, "Te": {}, "Trailer": {}, "Expect": {},
	"Proxy-Authorization": {}, "Authorization": {},
}

// ValidateHTTP returns a human-readable validation error for the HTTP options,
// or an empty string when they are valid.
func ValidateHTTP(o HTTPOptions) string {
	switch o.Method {
	case "", http.MethodGet, http.MethodHead, http.MethodPost:
	default:
		return "method must be GET, HEAD or POST"
	}
	if o.Body != "" && o.Method != http.MethodPost {
		return "body is only sent with POST"
	}
	if len(o.Body) > MaxBodyBytes {
		return fmt.Sprintf("body must be at most %d bytes", MaxBodyBytes)
	}
	if reason := validateHeaders(o.Headers); reason != "" {
		return reason
	}
	if reason := validateAuth(o.AuthType, o.AuthUsername, o.AuthSecret); reason != "" {
		return reason
	}
	if o.CAPEM != "" {
		if len(o.CAPEM) > MaxCAPEMBytes {
			return fmt.Sprintf("ca_pem must be at most %d bytes", MaxCAPEMBytes)
		}
		if !x509.NewCertPool().AppendCertsFromPEM([]byte(o.CAPEM)) {
			return "ca_pem has no valid PEM certificate"
		}
	}
	if len(o.AcceptedStatuses) > MaxStatusEntries {
		return fmt.Sprintf("accepted_statuses allows at most %d entries", MaxStatusEntries)
	}
	if _, err := ParseStatusRanges(o.AcceptedStatuses); err != nil {
		return err.Error()
	}
	switch o.KeywordMode {
	case "", KeywordContains:
	case KeywordNotContains:
		if o.Keyword == "" {
			return "keyword_mode not_contains needs keyword_check"
		}
	default:
		return "keyword_mode must be contains or not_contains"
	}
	if o.Keyword != "" && o.Method == http.MethodHead {
		return "keyword_check needs a response body, so it cannot be used with HEAD"
	}
	return ""
}

func validateHeaders(headers map[string]string) string {
	if len(headers) > MaxHeaders {
		return fmt.Sprintf("headers allows at most %d entries", MaxHeaders)
	}
	seen := make(map[string]struct{}, len(headers))
	for name, value := range headers {
		if !validHeaderName(name) {
			return fmt.Sprintf("header name %q is not valid", name)
		}
		canonical := http.CanonicalHeaderKey(name)
		if _, ok := reservedHeaders[canonical]; ok {
			if canonical == "Authorization" {
				return "set the Authorization header with auth, not headers"
			}
			return fmt.Sprintf("header %q cannot be set on a monitor", canonical)
		}
		if _, ok := seen[canonical]; ok {
			return fmt.Sprintf("header %q is set twice", canonical)
		}
		seen[canonical] = struct{}{}
		if len(value) > MaxHeaderValue {
			return fmt.Sprintf("header %q value must be at most %d bytes", canonical, MaxHeaderValue)
		}
		if !validHeaderValue(value) {
			return fmt.Sprintf("header %q value has a control character", canonical)
		}
	}
	return ""
}

func validateAuth(authType, username, secret string) string {
	switch authType {
	case "":
		return ""
	case AuthBasic:
		if username == "" {
			return "basic auth needs a username"
		}
		if strings.Contains(username, ":") {
			return "basic auth username cannot contain a colon"
		}
	case AuthBearer:
		if secret == "" {
			return "bearer auth needs a token"
		}
	default:
		return "auth type must be basic or bearer"
	}
	if len(username) > MaxAuthField || len(secret) > MaxAuthField {
		return fmt.Sprintf("auth fields must be at most %d bytes", MaxAuthField)
	}
	if !validHeaderValue(username) || !validHeaderValue(secret) {
		return "auth fields cannot contain control characters"
	}
	return ""
}

// validHeaderName reports whether name is an RFC 9110 token.
func validHeaderName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
		case strings.IndexByte("!#$%&'*+-.^_`|~", c) >= 0:
		default:
			return false
		}
	}
	return true
}

// validHeaderValue rejects control characters other than tab, so a value can
// never inject a CR or LF into the request.
func validHeaderValue(v string) bool {
	for i := 0; i < len(v); i++ {
		if c := v[i]; (c < 0x20 && c != '\t') || c == 0x7f {
			return false
		}
	}
	return true
}

// StatusRange is an inclusive range of HTTP status codes. A single code is a
// range with Lo == Hi.
type StatusRange struct {
	Lo, Hi int
}

// Contains reports whether code is inside the range.
func (r StatusRange) Contains(code int) bool { return code >= r.Lo && code <= r.Hi }

// String formats the range the way it is written in accepted_statuses.
func (r StatusRange) String() string {
	if r.Lo == r.Hi {
		return strconv.Itoa(r.Lo)
	}
	return fmt.Sprintf("%d-%d", r.Lo, r.Hi)
}

// ParseStatusRanges parses entries such as "200", "200-299" or "301". Codes
// must be within 100 to 599.
func ParseStatusRanges(entries []string) ([]StatusRange, error) {
	var out []StatusRange
	for _, e := range entries {
		e = strings.TrimSpace(e)
		lo, hi, isRange := strings.Cut(e, "-")
		from, err := parseStatusCode(lo)
		if err != nil {
			return nil, fmt.Errorf("accepted_statuses entry %q: %w", e, err)
		}
		to := from
		if isRange {
			if to, err = parseStatusCode(hi); err != nil {
				return nil, fmt.Errorf("accepted_statuses entry %q: %w", e, err)
			}
			if to < from {
				return nil, fmt.Errorf("accepted_statuses entry %q: range end is below its start", e)
			}
		}
		out = append(out, StatusRange{Lo: from, Hi: to})
	}
	return out, nil
}

func parseStatusCode(s string) (int, error) {
	code, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || code < 100 || code > 599 {
		return 0, fmt.Errorf("status must be a code from 100 to 599")
	}
	return code, nil
}
