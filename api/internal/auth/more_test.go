package auth

import (
	"strings"
	"testing"
)

func TestSessionAndTOTPHelpers(t *testing.T) {
	tok, err := NewSessionToken()
	if err != nil || len(tok) < 20 {
		t.Fatalf("NewSessionToken: %q %v", tok, err)
	}
	secret, uri, err := GenerateTOTP("admin@example.com")
	if err != nil || secret == "" || !strings.HasPrefix(uri, "otpauth://") {
		t.Fatalf("GenerateTOTP: %q %q %v", secret, uri, err)
	}
	dataURL, err := TOTPQRDataURL(uri)
	if err != nil || !strings.HasPrefix(dataURL, "data:image/png;base64,") {
		t.Fatalf("TOTPQRDataURL: %v", err)
	}
}
