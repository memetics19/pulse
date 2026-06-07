package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

// maintenanceView is the JSON shape returned to callers. AffectedMonitorIds is
// decoded from the stored JSON string into a []int64 for convenience.
type maintenanceView struct {
	ID                 int64     `json:"id"`
	Title              string    `json:"title"`
	Description        string    `json:"description"`
	Status             string    `json:"status"`
	AffectedMonitorIds []int64   `json:"affected_monitor_ids"`
	StartsAt           time.Time `json:"starts_at"`
	EndsAt             time.Time `json:"ends_at"`
}

// toView converts a generated.MaintenanceWindow into its API view. The stored
// affected_monitor_ids column holds a JSON array string; we decode it so
// callers receive a proper []int64 (falling back to an empty slice on error).
func toMaintenanceView(m generated.MaintenanceWindow) maintenanceView {
	var ids []int64
	if err := json.Unmarshal([]byte(m.AffectedMonitorIds), &ids); err != nil || ids == nil {
		ids = []int64{}
	}
	return maintenanceView{
		ID:                 m.ID,
		Title:              m.Title,
		Description:        m.Description,
		Status:             m.Status,
		AffectedMonitorIds: ids,
		StartsAt:           m.StartsAt,
		EndsAt:             m.EndsAt,
	}
}

// Maintenance handles CRUD for maintenance windows.
type Maintenance struct {
	q *generated.Queries
}

// NewMaintenance returns a new Maintenance handler backed by q.
func NewMaintenance(q *generated.Queries) *Maintenance {
	return &Maintenance{q: q}
}

// List returns all maintenance windows ordered by starts_at DESC.
func (h *Maintenance) List(w http.ResponseWriter, r *http.Request) {
	rows, err := h.q.ListMaintenance(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]maintenanceView, 0, len(rows))
	for _, m := range rows {
		views = append(views, toMaintenanceView(m))
	}
	writeJSON(w, http.StatusOK, views)
}

// Create accepts a new maintenance window and persists it. When start_now is
// true the status is set to "in_progress" and starts_at is overridden to now.
func (h *Maintenance) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Title              string  `json:"title"`
		Description        string  `json:"description"`
		AffectedMonitorIds []int64 `json:"affected_monitor_ids"`
		StartsAt           string  `json:"starts_at"`
		EndsAt             string  `json:"ends_at"`
		StartNow           bool    `json:"start_now"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	startsAt, err := time.Parse(time.RFC3339, input.StartsAt)
	if err != nil {
		http.Error(w, "invalid starts_at: "+err.Error(), http.StatusBadRequest)
		return
	}
	endsAt, err := time.Parse(time.RFC3339, input.EndsAt)
	if err != nil {
		http.Error(w, "invalid ends_at: "+err.Error(), http.StatusBadRequest)
		return
	}

	// Marshal []int64 back to a JSON string for the DB column.
	ids := input.AffectedMonitorIds
	if ids == nil {
		ids = []int64{}
	}
	idsJSON, err := json.Marshal(ids)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	status := "scheduled"
	if input.StartNow {
		status = "in_progress"
		startsAt = time.Now().UTC()
	}

	mw, err := h.q.CreateMaintenance(r.Context(), generated.CreateMaintenanceParams{
		Title:              input.Title,
		Description:        input.Description,
		Status:             status,
		AffectedMonitorIds: string(idsJSON),
		StartsAt:           startsAt,
		EndsAt:             endsAt,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusCreated, toMaintenanceView(mw))
}

// UpdateStatus changes the status of a maintenance window (e.g. to "cancelled").
func (h *Maintenance) UpdateStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var input struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := h.q.UpdateMaintenanceStatus(r.Context(), generated.UpdateMaintenanceStatusParams{
		Status: input.Status,
		ID:     id,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// Delete removes a maintenance window permanently.
func (h *Maintenance) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteMaintenance(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
