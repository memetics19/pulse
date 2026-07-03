// Package netguard blocks monitor checks from reaching internal networks.
// Monitor URLs are user-supplied, so without this guard an authenticated user
// could point a monitor at loopback, LAN, or cloud-metadata addresses (SSRF).
package netguard

import (
	"fmt"
	"net"
	"net/url"
	"syscall"
)

// cgnat is 100.64.0.0/10 (RFC 6598), not covered by net.IP.IsPrivate.
var cgnat = &net.IPNet{IP: net.IPv4(100, 64, 0, 0), Mask: net.CIDRMask(10, 32)}

// IsForbiddenIP reports whether ip points at an internal or special-purpose
// network: loopback, RFC 1918 private, link-local (incl. 169.254.169.254
// cloud metadata), unspecified, or CGNAT ranges.
func IsForbiddenIP(ip net.IP) bool {
	if v4 := ip.To4(); v4 != nil {
		ip = v4
	}
	return ip.IsLoopback() ||
		ip.IsPrivate() ||
		ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() ||
		cgnat.Contains(ip)
}

// DialControl returns a net.Dialer Control function that rejects connections
// to forbidden IPs. It runs on the literal IP after DNS resolution, on every
// connection (including redirects), so it also defeats DNS rebinding.
// With allowPrivate it permits everything.
func DialControl(allowPrivate bool) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, _ syscall.RawConn) error {
		if allowPrivate {
			return nil
		}
		host, _, err := net.SplitHostPort(address)
		if err != nil {
			return fmt.Errorf("netguard: %w", err)
		}
		ip := net.ParseIP(host)
		if ip == nil {
			return fmt.Errorf("netguard: %q is not an IP address", host)
		}
		if IsForbiddenIP(ip) {
			return fmt.Errorf("netguard: %s is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)", ip)
		}
		return nil
	}
}

// ValidateURL rejects monitor URLs that are malformed, use a non-HTTP scheme,
// or resolve to a forbidden IP. It is a best-effort early check for a clear
// API error; DialControl remains the enforcement point at connect time.
// A DNS lookup failure does not reject: the target may be temporarily
// unresolvable, and the dial-time guard still applies.
func ValidateURL(raw string, allowPrivate bool) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid url: %v", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("url scheme must be http or https, got %q", u.Scheme)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url has no host")
	}
	if allowPrivate {
		return nil
	}
	if ip := net.ParseIP(host); ip != nil {
		if IsForbiddenIP(ip) {
			return fmt.Errorf("%s is a private or internal address (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)", ip)
		}
		return nil
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil // unresolvable now; DialControl guards the actual connection
	}
	for _, ip := range ips {
		if IsForbiddenIP(ip) {
			return fmt.Errorf("%s resolves to private or internal address %s (set PULSE_ALLOW_PRIVATE_MONITORS=true to allow)", host, ip)
		}
	}
	return nil
}
