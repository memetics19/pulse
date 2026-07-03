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
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "reset-password" {
		runResetPassword(os.Args[2:])
		return
	}

	cfg := config.Load()
	dataDir := dataDir()

	boot, err := bootstrap.Load(dataDir)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	a := app.New()
	if boot.Configured {
		conn, err := db.Open(boot.SQLitePath)
		if err != nil {
			log.Fatalf("db: %v", err)
		}
		defer conn.Close()
		a.SetDB(conn)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	// Run the worker once the app is configured (now, or after setup completes).
	go func() {
		for {
			if a.Configured() {
				if err := worker.Run(ctx, a.DB(), cfg); err != nil {
					log.Printf("worker stopped: %v", err)
				}
				return
			}
			select {
			case <-ctx.Done():
				return
			case <-time.After(2 * time.Second):
			}
		}
	}()

	srv := server.New(a, dataDir, cfg)
	httpSrv := &http.Server{
		Addr:              ":" + cfg.Port,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Printf("pulse listening on :%s", cfg.Port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	log.Println("pulse shutting down")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// dataDir resolves the directory holding the bootstrap config and SQLite file.
func dataDir() string {
	if d := os.Getenv("PULSE_DATA_DIR"); d != "" {
		return d
	}
	if p := os.Getenv("SQLITE_PATH"); p != "" {
		return filepath.Dir(p)
	}
	return "/data"
}
