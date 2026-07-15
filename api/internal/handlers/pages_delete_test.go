package handlers_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPagesDelete(t *testing.T) {
	q := newQ(t)
	sp, err := q.CreateStatusPage(context.Background(), generated.CreateStatusPageParams{Domain: "d.com", Title: "D", Published: 1})
	require.NoError(t, err)
	rr := do(func(w http.ResponseWriter, r *http.Request) {
		handlers.NewPages(q).Delete(w, withChiID(r, "id", itoa(sp.ID)))
	}, "DELETE", "/x", nil)
	assert.Equal(t, http.StatusNoContent, rr.Code)
}
