package auth

import "testing"

func TestHashAndVerifyPassword(t *testing.T) {
	hash, err := HashPassword("correct horse battery staple")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if hash == "" || hash == "correct horse battery staple" {
		t.Fatal("hash must be non-empty and not the plaintext")
	}

	ok, err := VerifyPassword("correct horse battery staple", hash)
	if err != nil || !ok {
		t.Fatalf("verify correct password: ok=%v err=%v", ok, err)
	}

	bad, err := VerifyPassword("wrong", hash)
	if err != nil {
		t.Fatalf("verify wrong password errored: %v", err)
	}
	if bad {
		t.Fatal("wrong password must not verify")
	}
}
