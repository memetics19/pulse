package auth

import "github.com/alexedwards/argon2id"

// argonParams: OWASP-recommended, lightweight enough for a single rare admin login.
var argonParams = &argon2id.Params{
	Memory:      19 * 1024, // 19 MiB
	Iterations:  2,
	Parallelism: 1,
	SaltLength:  16,
	KeyLength:   32,
}

// HashPassword returns an encoded Argon2id hash (includes salt + params).
func HashPassword(plain string) (string, error) {
	return argon2id.CreateHash(plain, argonParams)
}

// VerifyPassword reports whether plain matches the encoded hash.
func VerifyPassword(plain, encodedHash string) (bool, error) {
	return argon2id.ComparePasswordAndHash(plain, encodedHash)
}
