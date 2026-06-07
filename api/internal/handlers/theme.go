package handlers

import (
	"encoding/json"
	"net/http"

	"github.com/memetics19/pulse/api/internal/generated"
)

type Theme struct {
	q *generated.Queries
}

func NewTheme(q *generated.Queries) *Theme {
	return &Theme{q: q}
}

func (h *Theme) Get(w http.ResponseWriter, r *http.Request) {
	theme, err := h.q.GetTheme(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(theme)
}

func (h *Theme) Update(w http.ResponseWriter, r *http.Request) {
	var params generated.UpsertThemeParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	theme, err := h.q.UpsertTheme(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(theme)
}
