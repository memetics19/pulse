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
	// sqliteDSN uses modernc.org/sqlite's _pragma=name(value) syntax (not the
	// mattn-style _journal_mode=WAL). WAL lets readers run concurrently with the
	// single writer; busy_timeout makes a contending connection wait rather than
	// fail with "database is locked". foreign_keys is per-connection, so it must
	// be in the DSN to apply to every pooled connection.
	dsn, err := sqliteDSN(sqlitePath)
	if err != nil {
		return nil, err
	}
	conn, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	// With WAL + busy_timeout, multiple connections are safe: readers (status
	// page, API GETs) no longer serialize behind the writer as they did under the
	// old single-connection pool. SQLite still allows only one writer at a time,
	// which busy_timeout serializes safely.
	// An in-memory database exists per connection, so it must stay on one.
	if isMemoryDB(sqlitePath) {
		conn.SetMaxOpenConns(1)
	} else {
		conn.SetMaxOpenConns(8)
		conn.SetMaxIdleConns(8)
	}
	if err := runMigrations(conn); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	if err := hashLegacyAgentTokens(conn); err != nil {
		return nil, errors.Join(err, conn.Close())
	}
	return conn, nil
}

func isMemoryDB(sqlitePath string) bool {
	return sqlitePath == ":memory:" || strings.Contains(sqlitePath, "mode=memory")
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
	// Rollback is a no-op once Commit succeeds. On an earlier failure the
	// original error is the one worth returning.
	defer func() { _ = tx.Rollback() }()

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
			return errors.Join(err, rows.Close())
		}
		pending = append(pending, l)
	}
	if err := rows.Err(); err != nil {
		return errors.Join(err, rows.Close())
	}
	if err := rows.Close(); err != nil {
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
