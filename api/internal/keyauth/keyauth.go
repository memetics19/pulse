package keyauth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
)

const keyPrefix = "pulse_live_"

// Generate returns (fullKey, displayPrefix, sha256hash). The full key is shown
// to the user once; only the hash is stored.
func Generate() (full, prefix, hash string, err error) {
	b := make([]byte, 24)
	if _, err = rand.Read(b); err != nil {
		return "", "", "", err
	}
	full = keyPrefix + base64.RawURLEncoding.EncodeToString(b)
	prefix = full[:len(keyPrefix)+6]
	hash = Hash(full)
	return full, prefix, hash, nil
}

// Hash returns the hex sha256 of a key. API keys are high-entropy, so a fast
// hash is appropriate (unlike user passwords, which use Argon2id).
func Hash(full string) string {
	sum := sha256.Sum256([]byte(full))
	return hex.EncodeToString(sum[:])
}
