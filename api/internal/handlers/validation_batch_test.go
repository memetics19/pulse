package handlers_test

import (
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
)

func TestCreateValidationErrors(t *testing.T) {
	q := newQ(t)
	// apikeys: missing name / empty scopes
	ak := handlers.NewAPIKeys(q)
	assert.Equal(t, http.StatusBadRequest, do(ak.Create, "POST", "/x", map[string]any{"scopes": []string{"monitors:read"}}).Code)
	assert.Equal(t, http.StatusBadRequest, do(ak.Create, "POST", "/x", "not json").Code)

	// groups: bad JSON
	g := handlers.NewGroups(q)
	assert.Equal(t, http.StatusBadRequest, do(g.Create, "POST", "/x", "not json").Code)

	// agents: bad JSON
	ag := handlers.NewAgents(q)
	assert.Equal(t, http.StatusBadRequest, do(ag.Create, "POST", "/x", "not json").Code)

	// incidents: bad JSON
	inc := handlers.NewIncidents(q)
	assert.Equal(t, http.StatusBadRequest, do(inc.Create, "POST", "/x", "not json").Code)

	// pages: bad JSON
	p := handlers.NewPages(q)
	assert.Equal(t, http.StatusBadRequest, do(p.Create, "POST", "/x", "not json").Code)
}
