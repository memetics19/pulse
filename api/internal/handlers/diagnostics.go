package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

// diagnosticsRequest is the POST /api/ingest/diagnostics body.
//
// Bundle is kept as raw JSON and stored verbatim: the agent owns the bundle
// schema, so collectors can be added or changed without a matching server-side
// migration. IncidentID is optional — an on-demand bundle from
// `pulse-agent --diagnose` describes a host at a moment, not an incident.
type diagnosticsRequest struct {
	IncidentID *int64          `json:"incident_id"`
	Bundle     json.RawMessage `json:"bundle"`
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

	var req diagnosticsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if len(req.Bundle) == 0 {
		http.Error(w, "bundle is required", http.StatusBadRequest)
		return
	}

	// Receipt time, not the agent's clock: it keeps ordering consistent across
	// agents with skewed clocks. The agent's own timestamp survives inside the
	// stored payload.
	if err := h.q.InsertIncidentDiagnostic(r.Context(), generated.InsertIncidentDiagnosticParams{
		IncidentID:  req.IncidentID,
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
