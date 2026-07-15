package worker

import (
	"context"
	"database/sql"
	"log"
	"time"

	"github.com/memetics19/pulse/api/internal/config"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
	"github.com/memetics19/pulse/api/internal/worker/checker"
	"github.com/memetics19/pulse/api/internal/worker/incident"
	"github.com/memetics19/pulse/api/internal/worker/maintenance"
	"github.com/memetics19/pulse/api/internal/worker/pruner"
	"github.com/memetics19/pulse/api/internal/worker/rollup"
	"github.com/memetics19/pulse/api/internal/worker/scheduler"
	"github.com/memetics19/pulse/api/store"
)

// Run starts the monitor scheduler plus the rollup/pruner loop and blocks
// until ctx is cancelled. beat, if non-nil, is called after each successful
// reconcile so the caller can track worker liveness (see /healthz).
func Run(ctx context.Context, db *sql.DB, cfg config.Config, beat func()) error {
	q := store.New(db)

	disp := alerter.NewDispatcher(q, cfg.ResendAPIKey, cfg.SlackWebhookURL)
	det := incident.NewDetector(q)

	sched := scheduler.New(db, disp, det, cfg.AllowPrivateMonitors)
	if beat != nil {
		sched.SetHeartbeat(beat)
	}
	sched.SetChecker("http", checker.NewHTTP(0, "", cfg.AllowPrivateMonitors))
	sched.SetChecker("tcp", checker.NewTCP(cfg.AllowPrivateMonitors))
	sched.SetChecker("dns", checker.NewDNS(cfg.AllowPrivateMonitors))
	sched.SetChecker("ssl", checker.NewSSL(cfg.AllowPrivateMonitors))
	sched.SetChecker("ping", checker.NewPing(cfg.AllowPrivateMonitors))

	if err := sched.Start(ctx); err != nil {
		return err
	}

	go func() {
		r := rollup.New(q)
		p := pruner.New(q)
		m := maintenance.New(q)
		tick := time.NewTicker(time.Minute)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				if err := r.Run(ctx); err != nil {
					log.Printf("rollup: %v", err)
				}
				if err := p.Run(ctx); err != nil {
					log.Printf("pruner: %v", err)
				}
				if err := m.Run(ctx, time.Now()); err != nil {
					log.Printf("maintenance: %v", err)
				}
			}
		}
	}()

	<-ctx.Done()
	return nil
}
