package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/memetics19/pulse/api/internal/bootstrap"
	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/db"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/internal/worker"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "reset-password":
			runResetPassword(os.Args[2:])
			return
		case "backup":
			runCommand(backupDatabase, os.Args[2:])
			return
		case "restore":
			runCommand(restoreDatabase, os.Args[2:])
			return
		}
	}

	cfg := config.Load()
	dataDir := config.DataDir()
	checkEmail(cfg)

	// The lock only tells pulse restore that the server runs. Without it the
	// server still starts, and restore still refuses while the database is
	// open (see backup.Restore).
	if lock, err := lockDataDir(dataDir); err != nil {
		log.Printf("could not lock %s: %v. pulse restore cannot tell that this server is running", lockPath(dataDir), err)
	} else {
		defer func() { _ = lock.Close() }()
	}

	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	a := app.New()
	conn, err := openDatabase(dataDir, boot, cfg)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	if conn != nil {
		defer func() {
			if err := conn.Close(); err != nil {
				log.Printf("db close: %v", err)
			}
		}()
		a.SetDB(conn)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := serve(ctx, a, dataDir, cfg); err != nil {
		log.Fatal(err)
	}
}

// checkEmail logs, at startup, why email alerts cannot be sent, and which transport
// wins when both SMTP and Resend are set up.
func checkEmail(cfg config.Config) {
	if cfg.SMTP.Host != "" && cfg.ResendAPIKey != "" {
		log.Printf("PULSE_SMTP_HOST and RESEND_API_KEY are both set: email alerts go through SMTP and RESEND_API_KEY is ignored")
	}
	if err := alerter.CheckEmail(cfg); err != nil {
		log.Printf("email alerts are set up but cannot be sent: %v", err)
	}
}

// serve runs the monitoring worker and the HTTP server until ctx is cancelled,
// then gracefully shuts the server down. It returns a non-nil error only if the
// server fails to start.
func serve(ctx context.Context, a *app.App, dataDir string, cfg config.Config) error {
	go runWorker(ctx, a, cfg)

	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           server.New(a, dataDir, cfg),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errc := make(chan error, 1)
	go func() {
		log.Printf("pulse listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errc <- err
		}
	}()

	select {
	case err := <-errc:
		return err
	case <-ctx.Done():
	}
	log.Println("pulse shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	return httpSrv.Shutdown(shutdownCtx)
}

// runWorker runs the worker once the app is configured (now, or after setup
// completes). If worker.Run returns an error (e.g. a transient DB error at
// startup), it retries with backoff instead of leaving monitoring permanently
// dead while the process still looks healthy — /healthz reflects the worker's
// liveness. It returns when ctx is cancelled.
func runWorker(ctx context.Context, a *app.App, cfg config.Config) {
	backoff := time.Second
	for {
		if a.Configured() {
			if err := worker.Run(ctx, a.DB(), cfg, a.MarkWorkerAlive); err != nil {
				log.Printf("worker stopped: %v; retrying in %s", err, backoff)
				select {
				case <-ctx.Done():
					return
				case <-time.After(backoff):
				}
				if backoff < 30*time.Second {
					backoff *= 2
				}
				continue
			}
			return // clean shutdown (ctx cancelled)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(2 * time.Second):
		}
	}
}

// openDatabase opens the database to serve. It returns nil when setup has not
// run yet. When pulse.json says setup ran, it opens the recorded file. When
// pulse.json is missing, it adopts the server's database file if that file
// already has an admin (see adoptDatabase).
func openDatabase(dataDir string, boot bootstrap.Config, cfg config.Config) (*sql.DB, error) {
	if boot.Configured {
		return db.Open(databaseFile(dataDir, boot, cfg))
	}
	return adoptDatabase(dataDir, cfg)
}

// adoptDatabase handles a database that outlived its pulse.json. That happens
// when pulse.json is lost, when a backup is restored without it, or when
// PULSE_DATA_DIR changes while PULSE_DB keeps pointing at the same file.
// Setup cannot finish on such a database, because it already has an admin.
//
// If the file the server would set up (cfg.SQLitePath, from PULSE_DB or
// PULSE_DATA_DIR) exists and has at least one user, Pulse adopts it: it
// records the file in pulse.json and serves it. The existing accounts stay
// the only way in, so adopting never creates or changes an account. A missing
// file or a file without users leaves the instance in setup mode, and a file
// that does not exist is not created.
func adoptDatabase(dataDir string, cfg config.Config) (*sql.DB, error) {
	path := cfg.SQLitePath
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	conn, err := db.Open(path)
	if err != nil {
		return nil, err
	}
	users, err := generated.New(conn).CountUsers(context.Background())
	if err != nil {
		return nil, errors.Join(fmt.Errorf("count users in %s: %w", path, err), conn.Close())
	}
	if users == 0 {
		return nil, conn.Close()
	}
	if err := bootstrap.Save(dataDir, bootstrap.Config{Configured: true, SQLitePath: path}); err != nil {
		log.Printf("adopted the database %s, which already has an admin. Could not write %s: %v. Pulse adopts it again on the next start",
			path, bootstrap.Path(dataDir), err)
		return conn, nil
	}
	log.Printf("adopted the database %s, which already has an admin. Recorded it in %s. Log in with an existing account",
		path, bootstrap.Path(dataDir))
	return conn, nil
}

// databaseFile returns the SQLite file that serve and reset-password open.
// Before setup it is cfg.SQLitePath: PULSE_DB, else pulse.db in
// PULSE_DATA_DIR, else /data/pulse.db. The setup wizard creates that file and
// records it in pulse.json. After setup the recorded file is used, so an
// install keeps the database it was set up with. If PULSE_DB names another
// file, that is logged, because Pulse does not move a database by itself.
func databaseFile(dataDir string, boot bootstrap.Config, cfg config.Config) string {
	if !boot.Configured || boot.SQLitePath == "" {
		return cfg.SQLitePath
	}
	if cfg.SQLitePathSet && filepath.Clean(cfg.SQLitePath) != filepath.Clean(boot.SQLitePath) {
		log.Printf("PULSE_DB is %s, but setup recorded the database %s in %s. Using the recorded file. "+
			"To use PULSE_DB, stop Pulse, move the file there and update sqlite_path in that file",
			cfg.SQLitePath, boot.SQLitePath, bootstrap.Path(dataDir))
	}
	return boot.SQLitePath
}
