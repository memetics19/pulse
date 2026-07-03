package handlers

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

type Agents struct {
	q *generated.Queries
}

func NewAgents(q *generated.Queries) *Agents {
	return &Agents{q: q}
}

func (h *Agents) List(w http.ResponseWriter, r *http.Request) {
	agents, err := h.q.ListAgents(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if agents == nil {
		agents = []generated.InfraAgent{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(agents)
}

func (h *Agents) Create(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name      string `json:"name"`
		HostLabel string `json:"host_label"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Generate 48-char hex token (24 random bytes → hex)
	tokenBytes := make([]byte, 24)
	if _, err := rand.Read(tokenBytes); err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	token := hex.EncodeToString(tokenBytes)

	// Only the sha256 hash is stored; the plaintext token is returned once
	// in this response and cannot be recovered afterwards.
	agent, err := h.q.CreateAgent(r.Context(), generated.CreateAgentParams{
		Name:      body.Name,
		HostLabel: body.HostLabel,
		TokenHash: keyauth.Hash(token),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(struct {
		generated.InfraAgent
		Token string `json:"token"`
	}{InfraAgent: agent, Token: token})
}

func (h *Agents) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteAgent(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (h *Agents) GetMetrics(w http.ResponseWriter, r *http.Request) {
	agentIDStr := chi.URLParam(r, "agentID")
	agentID, err := strconv.ParseInt(agentIDStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid agentID", http.StatusBadRequest)
		return
	}

	days := 1
	if dStr := r.URL.Query().Get("days"); dStr != "" {
		if d, err := strconv.Atoi(dStr); err == nil && d > 0 {
			days = d
		}
	}

	since := time.Now().AddDate(0, 0, -days)
	metrics, err := h.q.List1mMetrics(r.Context(), generated.List1mMetricsParams{
		AgentID:  agentID,
		BucketAt: since,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if metrics == nil {
		metrics = []generated.InfraMetrics1m{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(metrics)
}
