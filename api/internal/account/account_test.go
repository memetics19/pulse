package account

import (
	"context"
	"testing"

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
