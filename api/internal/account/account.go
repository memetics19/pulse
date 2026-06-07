package account

import (
	"context"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

// ResetPassword sets a new Argon2id password hash for the given username.
func ResetPassword(ctx context.Context, q *generated.Queries, username, newPassword string) error {
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	return q.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{
		PasswordHash: hash,
		Username:     username,
	})
}
