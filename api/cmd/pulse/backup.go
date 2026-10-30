package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/memetics19/pulse/api/internal/backup"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
)

// runCommand runs a subcommand that prints its result to stdout and exits
// with status 1 on error.
func runCommand(cmd func(args []string, w io.Writer) error, args []string) {
	if err := cmd(args, os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// backupDatabase writes a snapshot of the database the server opens (see
// databaseFile) while the server keeps running, plus a copy of pulse.json
// beside it. Without --out the snapshot goes next to the database.
func backupDatabase(args []string, w io.Writer) (err error) {
	fs := flag.NewFlagSet("backup", flag.ContinueOnError)
	out := fs.String("out", "", "snapshot file to write (default: pulse-backup-<time>.db beside the database)")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: pulse backup [--out <file>]")
	}
	cfg := config.Load()
	dataDir := config.DataDir()
	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	path := databaseFile(dataDir, boot, cfg)
	conn, err := backup.Open(path)
	if err != nil {
		return fmt.Errorf("no database to back up: %w", err)
	}
	defer func() { err = errors.Join(err, conn.Close()) }()
	if *out == "" {
		*out = filepath.Join(filepath.Dir(path), backup.Name(time.Now()))
	}
	res, err := backup.Snapshot(context.Background(), conn, *out, bootstrap.Path(dataDir))
	if err != nil {
		return fmt.Errorf("backup: %w", err)
	}
	if _, err := fmt.Fprintf(w, "backup written: %s (%d bytes, schema version %d)\n", res.Path, res.Size, res.Version); err != nil {
		return err
	}
	if res.Config != "" {
		_, err = fmt.Fprintf(w, "pulse.json copied to %s\n", res.Config)
	}
	return err
}

// restoreDatabase swaps a snapshot in for the database the server opens. It
// takes the instance lock first, so it refuses while the server runs.
func restoreDatabase(args []string, w io.Writer) error {
	fs := flag.NewFlagSet("restore", flag.ContinueOnError)
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() != 1 {
		return errors.New("usage: pulse restore <file>")
	}
	cfg := config.Load()
	dataDir := config.DataDir()
	lock, err := lockDataDir(dataDir)
	if errors.Is(err, errLocked) {
		return fmt.Errorf("pulse is running: another process holds %s. Stop the server, then run pulse restore again", lockPath(dataDir))
	}
	if err != nil {
		return fmt.Errorf("lock %s: %w", lockPath(dataDir), err)
	}
	defer func() { _ = lock.Close() }()
	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		return fmt.Errorf("bootstrap: %w", err)
	}
	target := databaseFile(dataDir, boot, cfg)
	res, err := backup.Restore(context.Background(), fs.Arg(0), target, time.Now())
	if err != nil {
		return fmt.Errorf("restore: %w", err)
	}
	msg := fmt.Sprintf("restored %s into %s (schema version %d)\n", fs.Arg(0), target, res.Version)
	if res.Kept != "" {
		msg += fmt.Sprintf("the old database is kept as %s\n", res.Kept)
	}
	if res.Version < res.Latest {
		msg += fmt.Sprintf("pulse migrates it to schema version %d when it starts\n", res.Latest)
	}
	_, err = io.WriteString(w, msg)
	return err
}
