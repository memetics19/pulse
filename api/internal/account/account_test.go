package account

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
	"github.com/memetics19/pulse/api/testutil"
)

func TestResetPasswordUpdatesHash(t *testing.T) {
	db := testutil.NewTestDB(t)
	q := generated.New(db)
	h, _ := auth.HashPassword("old-password")
	_, _ = q.CreateUser(context.Background(), generated.CreateUserParams{Username: "admin", PasswordHash: h})

	if err := ResetPassword(context.Background(), q, "admin", "brand-new-pass"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	u, _ := q.GetUserByUsername(context.Background(), "admin")
	ok, _ := auth.VerifyPassword("brand-new-pass", u.PasswordHash)
	if !ok {
		t.Fatal("password was not updated to the new value")
	}
}

// A password reset signs out every session of that user, since the reset is
// what you run when a session or the password may be compromised. Other
// users keep their sessions.
func TestResetPasswordSignsOutSessions(t *testing.T) {
	ctx := context.Background()
	q := generated.New(testutil.NewTestDB(t))
	h, _ := auth.HashPassword("old-password")
	admin, err := q.CreateUser(ctx, generated.CreateUserParams{Username: "admin", PasswordHash: h})
	if err != nil {
		t.Fatal(err)
	}
	other, err := q.CreateUser(ctx, generated.CreateUserParams{Username: "other", PasswordHash: h})
	if err != nil {
		t.Fatal(err)
	}
	for token, id := range map[string]int64{"a1": admin.ID, "a2": admin.ID, "o1": other.ID} {
		if err := q.CreateSession(ctx, generated.CreateSessionParams{Token: token, UserID: id, ExpiresAt: time.Now().Add(time.Hour)}); err != nil {
			t.Fatal(err)
		}
	}

	if err := ResetPassword(ctx, q, "admin", "brand-new-pass"); err != nil {
		t.Fatalf("reset: %v", err)
	}
	for _, token := range []string{"a1", "a2"} {
		if _, err := q.GetSession(ctx, token); err == nil {
			t.Fatalf("session %s survived the password reset", token)
		}
	}
	if _, err := q.GetSession(ctx, "o1"); err != nil {
		t.Fatalf("another user's session was removed: %v", err)
	}
}

func TestResetPasswordUnknownUser(t *testing.T) {
	q := generated.New(testutil.NewTestDB(t))
	err := ResetPassword(context.Background(), q, "nobody", "brand-new-pass")
	if err == nil || !strings.Contains(err.Error(), `no user named "nobody"`) {
		t.Fatalf("err = %v, want unknown user error", err)
	}
}

func TestResetPasswordDBErrors(t *testing.T) {
	ctx := context.Background()
	db := testutil.NewTestDB(t)
	h, _ := auth.HashPassword("old-password")
	if _, err := generated.New(db).CreateUser(ctx, generated.CreateUserParams{Username: "admin", PasswordHash: h}); err != nil {
		t.Fatal(err)
	}
	// Calls in order: GetUserByUsername, UpdateUserPassword, DeleteUserSessions.
	for n, want := range map[int]string{0: "", 1: "", 2: "sessions could not be signed out"} {
		err := ResetPassword(ctx, generated.New(testutil.FailAfter(db, n)), "admin", "brand-new-pass")
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("fail after %d: err = %v, want %q", n, err, want)
		}
	}
}
