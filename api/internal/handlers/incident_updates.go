package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

type IncidentUpdates struct {
	q      *generated.Queries
	notify Notifier
}

func NewIncidentUpdates(q *generated.Queries) *IncidentUpdates {
	return &IncidentUpdates{q: q}
}

// WithNotifier makes the handler alert the notification channels when an
// update is posted to an incident's timeline.
func (h *IncidentUpdates) WithNotifier(n Notifier) *IncidentUpdates {
	h.notify = n
	return h
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
	encodeJSON(w, updates)
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
	// The update is stored, so a failed lookup only costs the alert.
	if h.notify != nil {
		if inc, err := h.q.GetIncident(r.Context(), id); err == nil {
			h.notify.Notify(r.Context(), alerter.Alert{
				IncidentID: inc.ID,
				Title:      inc.Title,
				Status:     update.Status,
				Severity:   inc.Severity,
				Message:    update.Message,
			}, alerter.MonitorIDs(inc.AffectedMonitorIds)...)
		}
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	encodeJSON(w, update)
}
