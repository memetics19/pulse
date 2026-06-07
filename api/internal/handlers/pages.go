package handlers

import (
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/memetics19/pulse/api/internal/generated"
)

// Pages handles CRUD for status pages.
type Pages struct {
	q *generated.Queries
}

// NewPages returns a Pages handler backed by q.
func NewPages(q *generated.Queries) *Pages {
	return &Pages{q: q}
}

// pageView is the JSON shape returned to callers. int64 0/1 DB columns are
// surfaced as booleans and group associations are inlined as group_ids.
type pageView struct {
	ID        int64   `json:"id"`
	Domain    string  `json:"domain"`
	Title     string  `json:"title"`
	IsDefault bool    `json:"is_default"`
	Published bool    `json:"published"`
	GroupIDs  []int64 `json:"group_ids"`
}

// toPageView converts a generated.StatusPage into its API view.
func toPageView(sp generated.StatusPage, groupIDs []int64) pageView {
	if groupIDs == nil {
		groupIDs = []int64{}
	}
	return pageView{
		ID:        sp.ID,
		Domain:    sp.Domain,
		Title:     sp.Title,
		IsDefault: sp.IsDefault == 1,
		Published: sp.Published == 1,
		GroupIDs:  groupIDs,
	}
}

// b2i converts a boolean to the int64 SQLite representation used by sqlc.
func b2i(b bool) int64 {
	if b {
		return 1
	}
	return 0
}

// List returns all status pages ordered by is_default DESC, created_at ASC.
// Each page includes its associated group IDs.
func (h *Pages) List(w http.ResponseWriter, r *http.Request) {
	pages, err := h.q.ListStatusPages(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	views := make([]pageView, 0, len(pages))
	for _, sp := range pages {
		ids, err := h.q.ListPageGroupIDs(r.Context(), sp.ID)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		views = append(views, toPageView(sp, ids))
	}
	writeJSON(w, http.StatusOK, views)
}

// Create persists a new status page with optional group associations.
func (h *Pages) Create(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Domain    string  `json:"domain"`
		Title     string  `json:"title"`
		Published bool    `json:"published"`
		GroupIDs  []int64 `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if input.Title == "" {
		http.Error(w, "title is required", http.StatusBadRequest)
		return
	}

	sp, err := h.q.CreateStatusPage(r.Context(), generated.CreateStatusPageParams{
		Domain:    input.Domain,
		Title:     input.Title,
		Published: b2i(input.Published),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	for _, gid := range input.GroupIDs {
		if err := h.q.AddPageGroup(r.Context(), generated.AddPageGroupParams{
			StatusPageID: sp.ID,
			GroupID:      gid,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	writeJSON(w, http.StatusCreated, toPageView(sp, input.GroupIDs))
}

// Update replaces the fields of an existing status page and resets its groups.
func (h *Pages) Update(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	var input struct {
		Domain    string  `json:"domain"`
		Title     string  `json:"title"`
		Published bool    `json:"published"`
		GroupIDs  []int64 `json:"group_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	sp, err := h.q.GetStatusPage(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.q.UpdateStatusPage(r.Context(), generated.UpdateStatusPageParams{
		Domain:    input.Domain,
		Title:     input.Title,
		Published: b2i(input.Published),
		ID:        sp.ID,
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := h.q.ClearPageGroups(r.Context(), sp.ID); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, gid := range input.GroupIDs {
		if err := h.q.AddPageGroup(r.Context(), generated.AddPageGroupParams{
			StatusPageID: sp.ID,
			GroupID:      gid,
		}); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	// Re-fetch the updated row to return the canonical state.
	updated, err := h.q.GetStatusPage(r.Context(), sp.ID)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, http.StatusOK, toPageView(updated, input.GroupIDs))
}

// Delete removes a non-default status page. Returns 400 for the default page.
func (h *Pages) Delete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	sp, err := h.q.GetStatusPage(r.Context(), id)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if sp.IsDefault == 1 {
		http.Error(w, "cannot delete the default page", http.StatusBadRequest)
		return
	}

	if err := h.q.DeleteStatusPage(r.Context(), id); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
