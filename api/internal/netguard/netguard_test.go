package netguard

import (
	"context"
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

func TestValidateConnectionTargets(t *testing.T) {
	if err := ValidateHost("127.0.0.1", false); err == nil {
		t.Fatal("private ping host should be rejected")
	}
	if err := ValidateHostPort("127.0.0.1:443", false); err == nil {
		t.Fatal("private TCP/SSL target should be rejected")
	}
	if err := ValidateHost("127.0.0.1", true); err != nil {
		t.Fatalf("allowPrivate should permit private host: %v", err)
	}
	if err := ValidateHostPort("127.0.0.1:443", true); err != nil {
		t.Fatalf("allowPrivate should permit private host:port: %v", err)
	}
}

func TestResolveAllowedIP(t *testing.T) {
	ip, err := ResolveAllowedIP(context.Background(), "93.184.216.34", false)
	if err != nil {
		t.Fatalf("public literal rejected: %v", err)
	}
	if got := ip.String(); got != "93.184.216.34" {
		t.Fatalf("resolved IP = %q", got)
	}
	if _, err := ResolveAllowedIP(context.Background(), "127.0.0.1", false); err == nil {
		t.Fatal("private literal should be rejected")
	}
	if _, err := ResolveAllowedIP(context.Background(), "localhost", false); err == nil {
		t.Fatal("hostname resolving privately should be rejected")
	}
}
