package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

// diagnosticsRequest is the POST /api/ingest/diagnostics body.
//
// Bundle is kept as raw JSON and stored verbatim: the agent owns the bundle
// schema, so collectors can be added or changed without a matching server-side
// migration.
type diagnosticsRequest struct {
	Bundle json.RawMessage `json:"bundle"`
}

// PostDiagnostics stores a diagnostic bundle pushed by an agent. It
// authenticates with the same agent bearer token as PostMetrics.
func (h *Ingest) PostDiagnostics(w http.ResponseWriter, r *http.Request) {
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

	if !h.allowDiagnostic(agent.ID, time.Now()) {
		w.Header().Set("Retry-After", strconv.Itoa(int(minDiagnosticInterval.Seconds())))
		http.Error(w, "too many diagnostic uploads for this agent", http.StatusTooManyRequests)
		return
	}

	var req diagnosticsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	// A bundle is a JSON object. Section contents stay unvalidated — the agent
	// owns that schema — but null, numbers, strings, and arrays are garbage
	// nothing downstream can render.
	var envelope map[string]any
	if json.Unmarshal(req.Bundle, &envelope) != nil || envelope == nil {
		http.Error(w, "bundle must be a JSON object", http.StatusBadRequest)
		return
	}

	// Receipt time, not the agent's clock: it keeps ordering consistent across
	// agents with skewed clocks. The agent's own timestamp survives inside the
	// stored payload.
	if err := h.q.InsertAgentDiagnostic(r.Context(), generated.InsertAgentDiagnosticParams{
		AgentID:     agent.ID,
		CollectedAt: time.Now(),
		Payload:     string(req.Bundle),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.q.UpdateAgentLastSeen(r.Context(), agent.ID); err != nil {
		// Non-fatal: the bundle is already stored.
		log.Printf("failed to update agent last seen: %v", err)
	}

	w.WriteHeader(http.StatusNoContent)
}

// diagnosticView is one stored bundle as returned to a client. Bundle is raw
// JSON rather than a quoted string so callers parse the payload once.
type diagnosticView struct {
	ID          int64           `json:"id"`
	AgentID     int64           `json:"agent_id"`
	CollectedAt time.Time       `json:"collected_at"`
	Bundle      json.RawMessage `json:"bundle"`
}

// defaultDiagnosticLimit and maxDiagnosticLimit bound a history request.
//
// A bundle can approach the 1 MiB ingest cap, so the ceiling is deliberately
// low: fifty rows would materialise tens of megabytes of payload twice over —
// once by the driver and again as raw JSON — while holding the single SQLite
// connection. Reading recent evidence needs a handful of bundles, not a page.
const (
	defaultDiagnosticLimit = 3
	maxDiagnosticLimit     = 5
)

// ListDiagnostics returns an agent's recent diagnostic bundles, newest first.
// Without this, uploaded evidence is only reachable by opening the database —
// which defeats the reason for pushing it off the host at all.
func (h *Agents) ListDiagnostics(w http.ResponseWriter, r *http.Request) {
	agentID, err := strconv.ParseInt(chi.URLParam(r, "agentID"), 10, 64)
	if err != nil {
		http.Error(w, "invalid agentID", http.StatusBadRequest)
		return
	}

	limit := int64(defaultDiagnosticLimit)
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil && n > 0 {
			limit = min(n, maxDiagnosticLimit)
		}
	}

	rows, err := h.q.ListAgentDiagnostics(r.Context(), generated.ListAgentDiagnosticsParams{
		AgentID: agentID, Limit: limit,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	views := make([]diagnosticView, 0, len(rows))
	for _, row := range rows {
		views = append(views, diagnosticView{
			ID:          row.ID,
			AgentID:     row.AgentID,
			CollectedAt: row.CollectedAt,
			Bundle:      json.RawMessage(row.Payload),
		})
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(views)
}
