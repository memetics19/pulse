package db

import (
	"database/sql"
	"embed"
	"errors"
	"net/url"
	"strings"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	"github.com/memetics19/pulse/api/internal/keyauth"
	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

func Open(sqlitePath string) (*sql.DB, error) {
	dsn, err := sqliteDSN(sqlitePath)
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	conn.SetMaxOpenConns(1)
	if err := runMigrations(conn); err != nil {
		conn.Close()
		return nil, err
	}
	if err := hashLegacyAgentTokens(conn); err != nil {
		conn.Close()
		return nil, err
	}
	return conn, nil
}

func sqliteDSN(sqlitePath string) (string, error) {
	var parsed *url.URL
	var err error
	switch {
	case sqlitePath == ":memory:":
		parsed = &url.URL{Scheme: "file", Opaque: ":memory:"}
	case strings.HasPrefix(sqlitePath, "file:"):
		parsed, err = url.Parse(sqlitePath)
		if err != nil {
			return "", err
		}
	default:
		parsed = &url.URL{Scheme: "file", Path: sqlitePath}
	}

	options := parsed.Query()
	options.Add("_pragma", "journal_mode(WAL)")
	options.Add("_pragma", "foreign_keys(1)")
	options.Add("_pragma", "busy_timeout(5000)")
	parsed.RawQuery = options.Encode()
	return parsed.String(), nil
}

// hashLegacyAgentTokens hashes agent tokens that were stored in plaintext
// before migration 8. Plaintext tokens are exactly 48 hex chars (24 random
// bytes); sha256 hex is 64 chars, so the length identifies un-hashed rows.
// SQLite has no sha256 built-in, so this runs in Go. It is idempotent.
func hashLegacyAgentTokens(conn *sql.DB) error {
	tx, err := conn.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, token_hash FROM infra_agents WHERE length(token_hash) = 48`)
	if err != nil {
		return err
	}
	type legacy struct {
		id    int64
		token string
	}
	var pending []legacy
	for rows.Next() {
		var l legacy
		if err := rows.Scan(&l.id, &l.token); err != nil {
			rows.Close()
			return err
		}
		pending = append(pending, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}

	for _, l := range pending {
		if _, err := tx.Exec(`UPDATE infra_agents SET token_hash = ? WHERE id = ?`, keyauth.Hash(l.token), l.id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func runMigrations(conn *sql.DB) error {
	src, err := iofs.New(migrations, "migrations")
	if err != nil {
		return err
	}
	driver, err := newMigrationDriver(conn)
	if err != nil {
		return err
	}
	m, err := migrate.NewWithInstance("iofs", src, "sqlite", driver)
	if err != nil {
		return err
	}
	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return err
	}
	return nil
}
