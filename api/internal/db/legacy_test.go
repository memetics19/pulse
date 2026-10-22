package db_test

import (
	"strings"
	"testing"

	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

func TestHashLegacyAgentTokensOnReopen(t *testing.T) {
	path := t.TempDir() + "/legacy.db"
	conn, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	// Insert an agent whose token_hash is a 48-char legacy plaintext token.
	plaintext := strings.Repeat("a", 48)
	if _, err := conn.Exec(`INSERT INTO infra_agents (name, host_label, token_hash) VALUES ('h','web',?)`, plaintext); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	// Reopen: hashLegacyAgentTokens should rewrite it to a sha256 hex hash.
	conn2, err := db.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn2.Close()
	var stored string
	if err := conn2.QueryRow(`SELECT token_hash FROM infra_agents LIMIT 1`).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if stored != keyauth.Hash(plaintext) {
		t.Fatalf("legacy token not hashed on reopen: got %q", stored)
	}
}
