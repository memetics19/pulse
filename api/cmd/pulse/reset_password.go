package main

import (
	"context"
	"flag"
	"log"

	"github.com/memetics19/pulse/api/internal/account"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
)

func runResetPassword(args []string) {
	fs := flag.NewFlagSet("reset-password", flag.ExitOnError)
	username := fs.String("username", "", "admin username")
	password := fs.String("password", "", "new password (min 8 chars)")
	_ = fs.Parse(args)
	if *username == "" || len(*password) < 8 {
		log.Fatal("usage: pulse reset-password --username <name> --password <min-8-chars>")
	}
	// Prefer the bootstrap path written by the setup wizard; fall back to config.
	path := ""
	if boot, err := bootstrap.Load(dataDir()); err == nil && boot.Configured {
		path = boot.SQLitePath
	}
	if path == "" {
		path = config.Load().SQLitePath
	}
	if path == "" {
		path = "/data/pulse.db"
	}
	conn, err := db.Open(path)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer conn.Close()
	if err := account.ResetPassword(context.Background(), generated.New(conn), *username, *password); err != nil {
		log.Fatalf("reset-password: %v", err)
	}
	log.Printf("password updated for %q", *username)
}
