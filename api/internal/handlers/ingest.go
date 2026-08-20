package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

type Ingest struct {
	q *generated.Queries

	// lastDiagnostic throttles diagnostic uploads per agent. In memory is
	// enough: Pulse is a single process, and this exists to stop a runaway
	// loop rather than a determined caller. Bounded by the number of agents.
	mu             sync.Mutex
	lastDiagnostic map[int64]time.Time
}

func NewIngest(q *generated.Queries) *Ingest {
	return &Ingest{q: q, lastDiagnostic: make(map[int64]time.Time)}
}

// minDiagnosticInterval is the shortest gap between diagnostic uploads from one
// agent. Bundles approach the 1 MiB request cap, so a cron misfiring in a tight
// loop could outpace retention. Short enough not to impede a human running
// --diagnose twice.
const minDiagnosticInterval = 5 * time.Second

// allowDiagnostic reports whether an agent may upload now, recording the time
// when it may.
func (h *Ingest) allowDiagnostic(agentID int64, now time.Time) bool {
	h.mu.Lock()
	defer h.mu.Unlock()

	if last, ok := h.lastDiagnostic[agentID]; ok && now.Sub(last) < minDiagnosticInterval {
		return false
	}
	h.lastDiagnostic[agentID] = now
	return true
}

func extractBearerToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}
	return ""
}

func (h *Ingest) PostMetrics(w http.ResponseWriter, r *http.Request) {
	token := extractBearerToken(r)
	if token == "" {
		http.Error(w, "missing bearer token", http.StatusUnauthorized)
		return
	}

	agent, err := h.q.GetAgentByTokenHash(r.Context(), keyauth.Hash(token))
	if err != nil {
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	var params generated.InsertRawMetricParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params.AgentID = agent.ID
	if params.CollectedAt.IsZero() {
		params.CollectedAt = time.Now()
	}

	if err := h.q.InsertRawMetric(r.Context(), params); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.q.UpdateAgentLastSeen(r.Context(), agent.ID); err != nil {
		// Non-fatal: log but continue
		log.Printf("failed to update agent last seen: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}
