package auth

import (
	"bytes"
	"encoding/base64"

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

// ValidateTOTP reports whether code is currently valid for secret.
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
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
