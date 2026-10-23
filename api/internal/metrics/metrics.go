// Package metrics serves Pulse state in the Prometheus text format at
// /metrics. The endpoint is off by default. Labels carry monitor names, types
// and groups only, never targets such as URLs or hosts.
package metrics

import (
	"context"
	"crypto/subtle"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/app"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// scrapeTimeout bounds the database reads done for one scrape.
const scrapeTimeout = 5 * time.Second

// Settings controls whether /metrics is served and how it is protected.
type Settings struct {
	// Enabled mounts /metrics. Off by default.
	Enabled bool
	// Token, when set, requires "Authorization: Bearer <token>" on scrapes.
	Token string
}

// SettingsFromEnv reads PULSE_METRICS and PULSE_METRICS_TOKEN.
func SettingsFromEnv() Settings {
	enabled := false
	switch strings.ToLower(strings.TrimSpace(os.Getenv("PULSE_METRICS"))) {
	case "1", "true", "yes":
		enabled = true
	}
	return Settings{Enabled: enabled, Token: strings.TrimSpace(os.Getenv("PULSE_METRICS_TOKEN"))}
}

// checksTotal counts recorded check results since the process started. It is
// package level so the check recorder (worker and push endpoint) can feed it
// without plumbing a registry through every constructor.
var checksTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
	Name: "pulse_monitor_checks_total",
	Help: "Check results recorded since the process started, by monitor and result.",
}, []string{"monitor", "result"})

// ObserveCheck counts one recorded check result for the named monitor.
// result is the final status: up, degraded or down.
func ObserveCheck(monitor, result string) {
	checksTotal.WithLabelValues(monitor, result).Inc()
}

// Handler returns the /metrics handler: Pulse state plus the standard Go and
// process collectors, guarded by a bearer token when token is non-empty.
func Handler(a *app.App, token string) http.Handler {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		checksTotal,
		newStateCollector(a),
	)
	h := promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		// A failed database read drops only the affected metrics. The Go,
		// process and worker metrics are still served.
		ErrorHandling: promhttp.ContinueOnError,
		ErrorLog:      log.Default(),
	})
	if token == "" {
		return h
	}
	return requireBearer(token, h)
}

func requireBearer(token string, next http.Handler) http.Handler {
	want := []byte(token)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		scheme, cred, ok := strings.Cut(r.Header.Get("Authorization"), " ")
		if !ok || !strings.EqualFold(scheme, "Bearer") ||
			subtle.ConstantTimeCompare([]byte(strings.TrimSpace(cred)), want) != 1 {
			w.Header().Set("WWW-Authenticate", `Bearer realm="pulse-metrics"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

var monitorLabels = []string{"monitor", "type", "group"}

// stateCollector reads monitor and incident state from the database on each
// scrape: one query for all monitors and one count of open incidents.
type stateCollector struct {
	a         *app.App
	up        *prometheus.Desc
	response  *prometheus.Desc
	incidents *prometheus.Desc
	beat      *prometheus.Desc
}

func newStateCollector(a *app.App) *stateCollector {
	return &stateCollector{
		a: a,
		up: prometheus.NewDesc("pulse_monitor_up",
			"Latest check status per active monitor: 1 up, 0.5 degraded, 0 down. Absent until the first check.",
			monitorLabels, nil),
		response: prometheus.NewDesc("pulse_monitor_response_seconds",
			"Response time of the latest check per active monitor, in seconds. Absent when the check reported none.",
			monitorLabels, nil),
		incidents: prometheus.NewDesc("pulse_incidents_open",
			"Incidents that are not resolved.", nil, nil),
		beat: prometheus.NewDesc("pulse_worker_last_beat_timestamp_seconds",
			"Unix time of the monitoring worker's last successful reconcile. 0 before the first beat.", nil, nil),
	}
}

func (c *stateCollector) Describe(ch chan<- *prometheus.Desc) {
	ch <- c.up
	ch <- c.response
	ch <- c.incidents
	ch <- c.beat
}

func (c *stateCollector) Collect(ch chan<- prometheus.Metric) {
	beat := 0.0
	if t := c.a.WorkerLastBeat(); !t.IsZero() {
		beat = float64(t.UnixNano()) / 1e9
	}
	ch <- prometheus.MustNewConstMetric(c.beat, prometheus.GaugeValue, beat)

	q := c.a.Queries()
	if q == nil {
		// Setup has not completed, so there is no database to read yet.
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), scrapeTimeout)
	defer cancel()

	open, err := q.CountOpenIncidents(ctx)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.incidents, err)
	} else {
		ch <- prometheus.MustNewConstMetric(c.incidents, prometheus.GaugeValue, float64(open))
	}

	rows, err := q.ListMonitorMetrics(ctx)
	if err != nil {
		ch <- prometheus.NewInvalidMetric(c.up, err)
		return
	}
	// Monitor names are not unique. Prometheus rejects a scrape that repeats a
	// label set, so the first monitor (lowest id) wins and later ones are skipped.
	seen := make(map[[3]string]bool, len(rows))
	for _, m := range rows {
		key := [3]string{m.Name, m.Type, m.GroupName}
		if seen[key] {
			continue
		}
		seen[key] = true
		if m.LastStatus == nil {
			continue
		}
		if v, ok := statusValue(*m.LastStatus); ok {
			ch <- prometheus.MustNewConstMetric(c.up, prometheus.GaugeValue, v, key[:]...)
		}
		if m.LastResponseTimeMs != nil {
			ch <- prometheus.MustNewConstMetric(c.response, prometheus.GaugeValue,
				float64(*m.LastResponseTimeMs)/1000, key[:]...)
		}
	}
}

func statusValue(status string) (float64, bool) {
	switch status {
	case "up":
		return 1, true
	case "degraded":
		return 0.5, true
	case "down":
		return 0, true
	}
	return 0, false
}
