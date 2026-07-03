package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/keyauth"
)

type APIKeys struct{ q *generated.Queries }

func NewAPIKeys(q *generated.Queries) *APIKeys { return &APIKeys{q: q} }

type apiKeyView struct {
	ID         int64    `json:"id"`
	Name       string   `json:"name"`
	Prefix     string   `json:"prefix"`
	Scopes     []string `json:"scopes"`
	LastUsedAt *string  `json:"last_used_at"`
	CreatedAt  string   `json:"created_at"`
}

func toView(k generated.ApiKey) apiKeyView {
	var scopes []string
	_ = json.Unmarshal([]byte(k.Scopes), &scopes)
	if scopes == nil {
		scopes = []string{}
	}
	v := apiKeyView{
		ID:        k.ID,
		Name:      k.Name,
		Prefix:    k.Prefix,
		Scopes:    scopes,
		CreatedAt: k.CreatedAt.Format("2006-01-02 15:04"),
	}
	if k.LastUsedAt != nil {
		s := k.LastUsedAt.Format("2006-01-02 15:04")
		v.LastUsedAt = &s
	}
	return v
}

func (h *APIKeys) List(w http.ResponseWriter, r *http.Request) {
	keys, err := h.q.ListAPIKeys(r.Context())
	if err != nil {
		http.Error(w, "database error", http.StatusInternalServerError)
		return
	}
	out := []apiKeyView{}
	for _, k := range keys {
		out = append(out, toView(k))
	}
	writeJSON(w, http.StatusOK, out)
}

func (h *APIKeys) Create(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name   string   `json:"name"`
		Scopes []string `json:"scopes"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "name is required", http.StatusBadRequest)
		return
	}
	if req.Scopes == nil {
		req.Scopes = []string{}
	}
	full, prefix, hash, err := keyauth.Generate()
	if err != nil {
		http.Error(w, "key generation failed", http.StatusInternalServerError)
		return
	}
	scopesJSON, _ := json.Marshal(req.Scopes)
	k, err := h.q.CreateAPIKey(r.Context(), generated.CreateAPIKeyParams{
		Name:    req.Name,
		KeyHash: hash,
		Prefix:  prefix,
		Scopes:  string(scopesJSON),
	})
	if err != nil {
		http.Error(w, "could not create key", http.StatusInternalServerError)
		return
	}
	resp := toView(k)
	// Full key is returned exactly once at creation; never stored in plain text.
	writeJSON(w, http.StatusCreated, struct {
		apiKeyView
		Key string `json:"key"`
	}{apiKeyView: resp, Key: full})
}

func (h *APIKeys) Revoke(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.q.RevokeAPIKey(r.Context(), id); err != nil {
		http.Error(w, "could not revoke", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
