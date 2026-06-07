package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

type IncidentUpdates struct {
	q *generated.Queries
}

func NewIncidentUpdates(q *generated.Queries) *IncidentUpdates {
	return &IncidentUpdates{q: q}
}

func (h *IncidentUpdates) List(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "incidentID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid incidentID", http.StatusBadRequest)
		return
	}
	updates, err := h.q.ListIncidentUpdates(r.Context(), id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if updates == nil {
		updates = []generated.IncidentUpdate{}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(updates)
}

func (h *IncidentUpdates) Create(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "incidentID")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid incidentID", http.StatusBadRequest)
		return
	}
	var params generated.CreateIncidentUpdateParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params.IncidentID = id
	update, err := h.q.CreateIncidentUpdate(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(update)
}
