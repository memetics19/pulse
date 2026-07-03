package handlers

import (
	"encoding/json"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

type Ingest struct {
	q *generated.Queries
}

func NewIngest(q *generated.Queries) *Ingest {
	return &Ingest{q: q}
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
