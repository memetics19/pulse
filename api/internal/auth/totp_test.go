package auth

import (
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

func TestTOTPGenerateAndValidate(t *testing.T) {
	secret, uri, err := GenerateTOTP("admin")
	if err != nil {
		t.Fatal(err)
	}
	if secret == "" || uri == "" {
		t.Fatal("empty secret/uri")
	}
	code, err := totp.GenerateCode(secret, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if !ValidateTOTP(secret, code) {
		t.Fatal("freshly generated code should validate")
	}
	qr, err := TOTPQRDataURL(uri)
	if err != nil || len(qr) < 20 || qr[:10] != "data:image" {
		t.Fatalf("bad qr: %v", err)
	}
}
