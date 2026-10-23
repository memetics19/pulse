package auth

import (
	"bytes"
	"crypto/subtle"
	"encoding/base64"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/hotp"
	"github.com/pquerna/otp/totp"
	qrcode "github.com/skip2/go-qrcode"
)

const totpIssuer = "Pulse"

// GenerateTOTP creates a new secret + otpauth provisioning URI for the account.
func GenerateTOTP(account string) (secret, uri string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{Issuer: totpIssuer, AccountName: account})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// totpPeriod and totpSkew match what authenticator apps use: a new code every
// 30 seconds, and one step of clock drift accepted either side.
const (
	totpPeriod = 30
	totpSkew   = 1
)

// MatchTOTP reports whether code is valid for secret at now and, if it is,
// the time step it belongs to. The step lets callers accept each code only
// once. Every candidate code is compared with crypto/subtle, so the check does
// not leak how many leading digits matched.
func MatchTOTP(secret, code string, now time.Time) (step int64, ok bool) {
	if len(code) != otp.DigitsSix.Length() {
		return 0, false
	}
	current := now.Unix() / totpPeriod
	for s := current - totpSkew; s <= current+totpSkew; s++ {
		want, err := hotp.GenerateCodeCustom(secret, uint64(s), hotp.ValidateOpts{
			Digits:    otp.DigitsSix,
			Algorithm: otp.AlgorithmSHA1,
		})
		if err != nil {
			return 0, false
		}
		if subtle.ConstantTimeCompare([]byte(want), []byte(code)) == 1 {
			step, ok = s, true
		}
	}
	return step, ok
}

// TOTPQRDataURL renders the provisioning URI as a PNG data URL for scanning.
func TOTPQRDataURL(uri string) (string, error) {
	png, err := qrcode.Encode(uri, qrcode.Medium, 256)
	if err != nil {
		return "", err
	}
	var b bytes.Buffer
	b.WriteString("data:image/png;base64,")
	b.WriteString(base64.StdEncoding.EncodeToString(png))
	return b.String(), nil
}
