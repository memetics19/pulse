package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/worker/alerter"
)

type Notifications struct {
	q            *generated.Queries
	allowPrivate bool
}

// NewNotifications serves the notification API. allowPrivate is
// PULSE_ALLOW_PRIVATE_MONITORS: it decides whether a webhook may point at a
// private or internal address (see alerter.ValidateNotification).
func NewNotifications(q *generated.Queries, allowPrivate bool) *Notifications {
	return &Notifications{q: q, allowPrivate: allowPrivate}
}

func (h *Notifications) List(w http.ResponseWriter, r *http.Request) {
	notifications, err := h.q.ListNotifications(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if notifications == nil {
		notifications = []generated.Notification{}
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, notifications)
}

func (h *Notifications) Create(w http.ResponseWriter, r *http.Request) {
	var params generated.CreateNotificationParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := alerter.ValidateNotification(params.Channel, params.ConfigJson, h.allowPrivate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	notification, err := h.q.CreateNotification(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	encodeJSON(w, notification)
}

func (h *Notifications) Update(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	var params generated.UpdateNotificationParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if err := alerter.ValidateNotification(params.Channel, params.ConfigJson, h.allowPrivate); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	params.ID = id
	notification, err := h.q.UpdateNotification(r.Context(), params)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	encodeJSON(w, notification)
}

func (h *Notifications) Delete(w http.ResponseWriter, r *http.Request) {
	idStr := chi.URLParam(r, "id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteNotification(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
