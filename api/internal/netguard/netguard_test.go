package netguard

import (
	"net"
	"testing"
)

func TestIsForbiddenIP(t *testing.T) {
	cases := []struct {
		ip        string
		forbidden bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.5.5", true},
		{"192.168.1.1", true},
		{"169.254.169.254", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"::1", true},
		{"fe80::1", true},
		{"::ffff:127.0.0.1", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"2606:4700:4700::1111", false},
	}
	for _, c := range cases {
		ip := net.ParseIP(c.ip)
		if ip == nil {
			t.Fatalf("bad test IP %q", c.ip)
		}
		if got := IsForbiddenIP(ip); got != c.forbidden {
			t.Errorf("IsForbiddenIP(%s) = %v, want %v", c.ip, got, c.forbidden)
		}
	}
}

func TestDialControl(t *testing.T) {
	deny := DialControl(false)
	if err := deny("tcp4", "127.0.0.1:80", nil); err == nil {
		t.Error("loopback should be rejected")
	}
	if err := deny("tcp4", "8.8.8.8:443", nil); err != nil {
		t.Errorf("public IP rejected: %v", err)
	}

	allow := DialControl(true)
	if err := allow("tcp4", "127.0.0.1:80", nil); err != nil {
		t.Errorf("allowPrivate should permit loopback: %v", err)
	}
}

func TestValidateURL(t *testing.T) {
	cases := []struct {
		url     string
		wantErr bool
	}{
		{"http://127.0.0.1/x", true},
		{"http://10.1.2.3:8080/", true},
		{"http://169.254.169.254/latest/meta-data/", true},
		{"http://[::1]/", true},
		{"file:///etc/passwd", true},
		{"not a url at all://", true},
		{"http://", true},
		{"https://example.com/health", false},
		{"http://93.184.216.34/", false},
	}
	for _, c := range cases {
		err := ValidateURL(c.url, false)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateURL(%q) error = %v, wantErr %v", c.url, err, c.wantErr)
		}
	}

	if err := ValidateURL("http://127.0.0.1/x", true); err != nil {
		t.Errorf("allowPrivate should permit loopback URL: %v", err)
	}
}

func TestValidateTarget(t *testing.T) {
	cases := []struct {
		target  string
		wantErr bool
	}{
		{"127.0.0.1:6379", true},     // loopback host:port
		{"10.0.0.5:5432", true},      // RFC1918 host:port
		{"169.254.169.254:80", true}, // cloud metadata
		{"192.168.1.1", true},        // bare private IP
		{"[::1]:443", true},          // IPv6 loopback host:port
		{"", true},                   // no host
		{"8.8.8.8:53", false},        // public host:port
		{"1.1.1.1", false},           // public bare IP
		{"this-domain-should-not-exist-pulse.invalid:80", false}, // unresolvable → dial guard covers
	}
	for _, c := range cases {
		err := ValidateTarget(c.target, false)
		if (err != nil) != c.wantErr {
			t.Errorf("ValidateTarget(%q) error = %v, wantErr %v", c.target, err, c.wantErr)
		}
	}
	if err := ValidateTarget("127.0.0.1:22", true); err != nil {
		t.Errorf("allowPrivate should permit loopback target: %v", err)
	}
}

func TestDialControlEdges(t *testing.T) {
	allow := DialControl(true)
	if err := allow("tcp", "127.0.0.1:80", nil); err != nil {
		t.Errorf("allowPrivate should permit: %v", err)
	}
	deny := DialControl(false)
	if err := deny("tcp", "not-an-address", nil); err == nil {
		t.Error("malformed address should error")
	}
	if err := deny("tcp", "example.com:80", nil); err == nil {
		t.Error("non-IP host (unresolved literal) should error")
	}
	if err := deny("tcp", "8.8.8.8:53", nil); err != nil {
		t.Errorf("public IP should be allowed: %v", err)
	}
}

func TestValidateURLEdges(t *testing.T) {
	if err := ValidateURL("://bad", false); err == nil {
		t.Error("malformed URL should error")
	}
	if err := ValidateURL("ftp://example.com", false); err == nil {
		t.Error("non-http scheme should error")
	}
	if err := ValidateURL("http://", false); err == nil {
		t.Error("missing host should error")
	}
}
