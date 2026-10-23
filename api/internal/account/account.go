package account

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/memetics19/pulse/api/internal/auth"
	"github.com/memetics19/pulse/api/internal/generated"
)

// ResetPassword sets a new Argon2id password hash for the given username and
// signs out every session of that user, so a session opened with the old
// password (for example a stolen one) does not outlive the reset. An unknown
// username is an error rather than a silent no-op.
func ResetPassword(ctx context.Context, q *generated.Queries, username, newPassword string) error {
	u, err := q.GetUserByUsername(ctx, username)
	if errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("no user named %q", username)
	}
	if err != nil {
		return err
	}
	hash, err := auth.HashPassword(newPassword)
	if err != nil {
		return err
	}
	if err := q.UpdateUserPassword(ctx, generated.UpdateUserPasswordParams{
		PasswordHash: hash,
		Username:     username,
	}); err != nil {
		return err
	}
	if _, err := q.DeleteUserSessions(ctx, u.ID); err != nil {
		return fmt.Errorf("password changed, but sessions could not be signed out: %w", err)
	}
	return nil
}
