package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

type Incidents struct {
	q *generated.Queries
}

func NewIncidents(q *generated.Queries) *Incidents {
	return &Incidents{q: q}
}

// canonicalMonitorIDs normalizes the incoming affected_monitor_ids value into a
// canonical JSON array of ints with no spaces (e.g. "[1,2,3]"). It accepts
// either a JSON array or a JSON string containing an array, so callers that
// double-encode the field still work. The canonical form is what the worker's
// substring-based containsMonitorID dedup expects. On any parse failure it
// falls back to "[]".
func canonicalMonitorIDs(raw json.RawMessage) string {
	if len(raw) == 0 {
		return "[]"
	}
	var ids []int64
	if err := json.Unmarshal(raw, &ids); err != nil {
		// Maybe the array was sent as a JSON string, e.g. "\"[1,2]\"".
		var s string
		if json.Unmarshal(raw, &s) != nil || json.Unmarshal([]byte(s), &ids) != nil {
			return "[]"
		}
	}
	if ids == nil {
		return "[]"
	}
	out, err := json.Marshal(ids)
	if err != nil {
		return "[]"
	}
	return string(out)
}

func (h *Incidents) List(w http.ResponseWriter, r *http.Request) {
	incidents, err := h.q.ListIncidents(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if incidents == nil {
		incidents = []generated.Incident{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incidents)
}

func (h *Incidents) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title              string          `json:"title"`
		Severity           string          `json:"severity"`
		AffectedMonitorIds json.RawMessage `json:"affected_monitor_ids"`
		Source             string          `json:"source"`
		ExternalID         string          `json:"external_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params := generated.CreateIncidentParams{
		Title:              input.Title,
		Severity:           input.Severity,
		AffectedMonitorIds: canonicalMonitorIDs(input.AffectedMonitorIds),
		StartedAt:          time.Now().UTC(),
		Source:             input.Source,
		ExternalID:         input.ExternalID,
	}
	incident, err := h.q.CreateIncident(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(incident)
}

func (h *Incidents) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var input struct {
		Status string `json:"status"`
		Rca    string `json:"rca"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// RCA enforcement: resolving without RCA is not allowed
	if input.Status == "resolved" && strings.TrimSpace(input.Rca) == "" {
		http.Error(w, "rca is required when resolving an incident", http.StatusBadRequest)
		return
	}

	params := generated.UpdateIncidentStatusParams{
		Status: input.Status,
		ID:     id,
	}

	if input.Status == "resolved" {
		now := time.Now().UTC()
		params.ResolvedAt = &now
		rca := input.Rca
		params.Rca = &rca
	}

	incident, err := h.q.UpdateIncidentStatus(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(incident)
}

func (h *Incidents) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteIncident(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
