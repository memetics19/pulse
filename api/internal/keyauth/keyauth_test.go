package keyauth

import "testing"

func TestGenerateAndHash(t *testing.T) {
	full, prefix, hash, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if len(full) < 20 || prefix == "" || hash == "" {
		t.Fatalf("bad outputs: full=%q prefix=%q", full, prefix)
	}
	if Hash(full) != hash {
		t.Fatal("Hash(full) must equal the returned hash")
	}
	if len(full) < 11 || full[:11] != "pulse_live_" {
		t.Fatalf("key should start with pulse_live_: %q", full)
	}
	full2, _, _, _ := Generate()
	if full2 == full {
		t.Fatal("keys must be unique")
	}
}
