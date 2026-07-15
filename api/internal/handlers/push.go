package handlers

import (
	"context"
	"net/http"
	"strconv"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
	pushauth "github.com/memetics19/pulse/api/internal/push"
	"github.com/memetics19/pulse/api/internal/worker/checkresult"
	"github.com/memetics19/pulse/api/store"
)

const maxPushMessageBytes = 1024
const maxPushPing = int64(100_000_000_000)

type checkResultRecorder interface {
	Record(context.Context, store.Monitor, checkresult.Input) error
}

type Push struct {
	q        *generated.Queries
	recorder checkResultRecorder
}

func NewPush(q *generated.Queries, recorder checkResultRecorder) *Push {
	return &Push{q: q, recorder: recorder}
}

func (h *Push) Heartbeat(w http.ResponseWriter, r *http.Request) {
	token := chi.URLParam(r, "token")
	if !pushauth.ValidToken(token) {
		pushNotFound(w)
		return
	}

	credential, err := h.q.GetPushTokenByHash(r.Context(), pushauth.HashToken(token))
	if err != nil {
		pushNotFound(w)
		return
	}
	monitor, err := h.q.GetMonitor(r.Context(), credential.MonitorID)
	if err != nil || monitor.Type != "push" || !monitor.IsActive {
		pushNotFound(w)
		return
	}

	status := r.URL.Query().Get("status")
	if status == "" {
		status = "up"
	}
	if status != "up" && status != "down" && status != "degraded" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid status"})
		return
	}
	message := r.URL.Query().Get("msg")
	if !utf8.ValidString(message) || len(message) > maxPushMessageBytes {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid message"})
		return
	}

	var ping *int64
	if r.URL.Query().Has("ping") {
		parsed, err := strconv.ParseInt(r.URL.Query().Get("ping"), 10, 64)
		if err != nil || parsed < 0 || parsed > maxPushPing {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid ping"})
			return
		}
		ping = &parsed
	}

	if h.recorder == nil || h.recorder.Record(r.Context(), monitor, checkresult.Input{
		Status: status, ResponseTimeMs: ping, ErrorMessage: message, CheckedAt: time.Now(),
	}) != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "could not record heartbeat"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func pushNotFound(w http.ResponseWriter) {
	writeJSON(w, http.StatusNotFound, map[string]string{"error": "push monitor not found"})
}
