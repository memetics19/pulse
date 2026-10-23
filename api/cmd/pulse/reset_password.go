package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"

	"github.com/memetics19/pulse/api/internal/account"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

func runResetPassword(args []string) {
	if err := resetPassword(args); err != nil {
		log.Fatal(err)
	}
}

// resetPassword sets a new password for an admin, in the same database file
// the server opens (see databaseFile).
func resetPassword(args []string) error {
	fs := flag.NewFlagSet("reset-password", flag.ContinueOnError)
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "new password (min 8 chars)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if *username == "" || len(*password) < 8 {
		return errors.New("usage: pulse reset-password --username <name> --password <min-8-chars>")
	}
	cfg := config.Load()
	dataDir := config.DataDir()
	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	conn, err := db.Open(databaseFile(dataDir, boot, cfg))
	if err != nil {
		return fmt.Errorf("db: %w", err)
	}
	defer func() {
		if err := conn.Close(); err != nil {
			log.Printf("db close: %v", err)
		}
	}()
	if err := account.ResetPassword(context.Background(), generated.New(conn), *username, *password); err != nil {
		return fmt.Errorf("reset-password: %w", err)
	}
	log.Printf("password updated for %q", *username)
	return nil
}
