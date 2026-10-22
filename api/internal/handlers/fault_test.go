package handlers_test

import (
	"net/http"
	"testing"

	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/internal/handlers"
	"github.com/memetics19/pulse/api/testutil"
	"github.com/stretchr/testify/assert"
)

// Overview issues four queries in sequence; failing after 1/2/3 successful
// calls drives each subsequent "database error" branch.
func TestOverviewDBErrorAtEachStage(t *testing.T) {
	for _, k := range []int{1, 2, 3} {
		db := testutil.NewTestDB(t)
		q := generated.New(testutil.FailAfter(db, k))
		rr := do(handlers.NewOverview(q).Get, "GET", "/api/overview", nil)
		assert.Equal(t, http.StatusInternalServerError, rr.Code, "overview fail after %d", k)
	}
}

// pages.Create inserts the page (call 1) then adds each group (later calls);
// failing after the insert exercises the group-association error branch.
func TestPagesCreateGroupAssocDBError(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed := generated.New(db)
	g, err := seed.CreateGroup(t.Context(), generated.CreateGroupParams{Name: "g"})
	if err != nil {
		t.Fatal(err)
	}
	// Fail after the status-page insert so AddPageGroup errors.
	q := generated.New(testutil.FailAfter(db, 1))
	rr := do(handlers.NewPages(q).Create, "POST", "/api/pages",
		map[string]any{"domain": "a.com", "title": "A", "group_ids": []int64{g.ID}})
	assert.Equal(t, http.StatusInternalServerError, rr.Code)
}

// auth.Setup: CountUsers (1) -> CreateUser (2) -> CreateSession (3). Failing
// after each stage drives the corresponding error branch.
func TestAuthSetupDBErrorStages(t *testing.T) {
	for _, k := range []int{1, 2} {
		db := testutil.NewTestDB(t)
		q := generated.New(testutil.FailAfter(db, k))
		body := map[string]any{"username": "admin", "password": "s3cret-pass"}
		rr := do(handlers.NewAuth(q, false).Setup, "POST", "/api/auth/setup", body)
		assert.Equal(t, http.StatusInternalServerError, rr.Code, "auth.Setup fail after %d", k)
	}
}

// auth.Login: GetUserByUsername (1) -> startSession/CreateSession (2). With
// valid creds, failing after the lookup drives the session-creation error.
func TestAuthLoginSessionDBError(t *testing.T) {
	db := testutil.NewTestDB(t)
	seed := generated.New(db)
	// Create the admin via a normal handler so credentials are valid.
	setupAdminExt(t, handlers.NewAuth(seed, false))

	q := generated.New(testutil.FailAfter(db, 1))
	body := map[string]any{"username": "admin", "password": "s3cret-pass"}
	rr := do(handlers.NewAuth(q, false).Login, "POST", "/api/auth/login", body)
	assert.NotEqual(t, http.StatusOK, rr.Code)
}
