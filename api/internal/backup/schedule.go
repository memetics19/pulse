package backup

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// autoPrefix names the snapshots the server takes by itself. Retention only
// counts and removes files with this prefix, so a backup taken by hand in the
// same directory stays.
const autoPrefix = "pulse-auto-"

// Interval is how old the newest automatic snapshot gets before the next one.
const Interval = 24 * time.Hour

// retryDelay is how long Schedule waits after a failed snapshot, so a full
// disk is logged once an hour rather than every minute.
const retryDelay = time.Hour

// Schedule takes the daily snapshots in PULSE_BACKUP_DIR.
type Schedule struct {
	conn       *sql.DB
	dir        string
	keep       int
	configFile string
	retryAt    time.Time
}

// NewSchedule returns a Schedule that snapshots conn into dir, copies
// configFile (pulse.json) beside each snapshot, and keeps the newest keep.
func NewSchedule(conn *sql.DB, dir string, keep int, configFile string) *Schedule {
	return &Schedule{conn: conn, dir: dir, keep: max(keep, 1), configFile: configFile}
}

// Run takes a snapshot when the newest one in the directory is a day old, or
// there is none, and then removes all but the newest keep. The worker calls
// it every minute.
func (s *Schedule) Run(ctx context.Context, now time.Time) error {
	if now.Before(s.retryAt) {
		return nil
	}
	if err := s.run(ctx, now); err != nil {
		s.retryAt = now.Add(retryDelay)
		return err
	}
	return nil
}

func (s *Schedule) run(ctx context.Context, now time.Time) error {
	if err := os.MkdirAll(s.dir, 0o750); err != nil {
		return err
	}
	names, err := s.snapshots()
	if err != nil {
		return err
	}
	if n := len(names); n > 0 {
		if age := now.Sub(snapshotTime(names[n-1])); age >= 0 && age < Interval {
			return nil
		}
	}
	name := autoPrefix + now.UTC().Format(timeLayout) + ".db"
	res, err := Snapshot(ctx, s.conn, filepath.Join(s.dir, name), s.configFile)
	if err != nil {
		return err
	}
	log.Printf("backup: wrote %s (%d bytes)", res.Path, res.Size)
	return s.prune(append(names, name))
}

// snapshots lists the automatic snapshots in the directory, oldest first.
func (s *Schedule) snapshots() ([]string, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && !snapshotTime(e.Name()).IsZero() {
			names = append(names, e.Name())
		}
	}
	// ReadDir sorts by name, and the fixed-width stamp sorts in time order.
	return names, nil
}

// snapshotTime returns the time in an automatic snapshot's name, or the zero
// time for any other file.
func snapshotTime(name string) time.Time {
	stamp, ok := strings.CutPrefix(name, autoPrefix)
	if !ok {
		return time.Time{}
	}
	stamp, ok = strings.CutSuffix(stamp, ".db")
	if !ok {
		return time.Time{}
	}
	t, err := time.Parse(timeLayout, stamp)
	if err != nil {
		return time.Time{}
	}
	return t
}

// prune removes all but the newest keep snapshots in names, with their copies
// of pulse.json.
func (s *Schedule) prune(names []string) error {
	if len(names) <= s.keep {
		return nil
	}
	var errs []error
	for _, name := range names[:len(names)-s.keep] {
		path := filepath.Join(s.dir, name)
		for _, p := range []string{path, ConfigPath(path)} {
			if err := os.Remove(p); err != nil && !errors.Is(err, fs.ErrNotExist) {
				errs = append(errs, fmt.Errorf("remove old snapshot: %w", err))
			}
		}
	}
	return errors.Join(errs...)
}
