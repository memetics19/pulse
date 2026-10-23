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
	if _, ok := MatchTOTP(secret, code, time.Now()); !ok {
		t.Fatal("freshly generated code should validate")
	}
	qr, err := TOTPQRDataURL(uri)
	if err != nil || len(qr) < 20 || qr[:10] != "data:image" {
		t.Fatalf("bad qr: %v", err)
	}
}

// MatchTOTP accepts the current step and one step of drift either side, and
// reports which step matched so callers can refuse a repeat.
func TestMatchTOTPSteps(t *testing.T) {
	secret, _, err := GenerateTOTP("admin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_800_000_015, 0) // mid-step, so ±30s stays one step away
	current := now.Unix() / 30
	for offset := int64(-1); offset <= 1; offset++ {
		code, err := totp.GenerateCode(secret, now.Add(time.Duration(offset)*30*time.Second))
		if err != nil {
			t.Fatal(err)
		}
		step, ok := MatchTOTP(secret, code, now)
		if !ok || step != current+offset {
			t.Fatalf("offset %d: step=%d ok=%v, want step %d", offset, step, ok, current+offset)
		}
	}
	far, _ := totp.GenerateCode(secret, now.Add(2*time.Minute))
	if _, ok := MatchTOTP(secret, far, now); ok {
		t.Fatal("a code four steps ahead must not match")
	}
	for _, bad := range []string{"", "12345", "1234567", "abcdef"} {
		if _, ok := MatchTOTP(secret, bad, now); ok {
			t.Fatalf("%q must not match", bad)
		}
	}
	if _, ok := MatchTOTP("not base32 !!", "123456", now); ok {
		t.Fatal("an undecodable secret must not match")
	}
}
