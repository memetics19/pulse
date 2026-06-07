package handlers

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/memetics19/pulse/api/internal/generated"
)

const defaultRetentionDays = 90

// Settings handles admin-configurable application settings.
type Settings struct{ q *generated.Queries }

func NewSettings(q *generated.Queries) *Settings { return &Settings{q: q} }

// Get returns the current retention_days setting (defaults to 90 if not set).
func (s *Settings) Get(w http.ResponseWriter, r *http.Request) {
	days := defaultRetentionDays
	if v, err := s.q.GetSetting(r.Context(), "retention_days"); err == nil {
		if n, e := strconv.Atoi(v); e == nil && n > 0 {
			days = n
		}
	}
	writeJSON(w, http.StatusOK, map[string]int{"retention_days": days})
}

// Update sets retention_days; the value must be >= 1.
func (s *Settings) Update(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RetentionDays int `json:"retention_days"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.RetentionDays < 1 {
		http.Error(w, "retention_days must be >= 1", http.StatusBadRequest)
		return
	}
	if err := s.q.SetSetting(r.Context(), generated.SetSettingParams{
		Key:   "retention_days",
		Value: strconv.Itoa(req.RetentionDays),
	}); err != nil {
		http.Error(w, "could not save", http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{"retention_days": req.RetentionDays})
}
