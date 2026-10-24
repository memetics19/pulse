package main

import (
	"context"
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
	"github.com/memetics19/pulse/api/internal/server"
	"github.com/memetics19/pulse/api/internal/worker"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		runResetPassword(os.Args[2:])
		return
	}

	cfg := config.Load()
	dataDir := config.DataDir()
	if cfg.ResendAPIKey != "" {
		if err := alerter.CheckEmailFrom(cfg.EmailFrom); err != nil {
			log.Printf("RESEND_API_KEY is set, but email alerts cannot be sent: %v", err)
		}
	}

	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	a := app.New()
	if boot.Configured {
		conn, err := db.Open(databaseFile(dataDir, boot, cfg))
		if err != nil {
			log.Fatalf("db: %v", err)
		}
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
