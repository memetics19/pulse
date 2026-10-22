package push

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"io"
	"regexp"
)

const tokenBytes = 24

var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{10,128}$`)

func GenerateToken() (string, error) {
	raw := make([]byte, tokenBytes)
	if _, err := io.ReadFull(rand.Reader, raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func ValidToken(token string) bool {
	return tokenPattern.MatchString(token)
}

func HashToken(token string) string {
	digest := sha256.Sum256([]byte(token))
	return hex.EncodeToString(digest[:])
}

func Prefix(token string) string {
	if len(token) <= 8 {
		return token
	}
	return token[:8]
}
