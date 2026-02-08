package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// AuthenticatedRole is the Postgres role used for authenticated API requests.
// SET LOCAL ROLE switches to this role within each request transaction so
// RLS policies are enforced even when the pool connects as a superuser.
const AuthenticatedRole = "ayb_authenticated"

// SetRLSContext switches to the authenticated role and sets Postgres session
// variables for RLS policies within the given transaction. Uses SET LOCAL
// and set_config(..., true), both scoped to the current transaction.
//
// Users write standard RLS policies referencing these variables:
//
//	CREATE POLICY user_owns_row ON posts
//	    USING (author_id::text = current_setting('ayb.user_id', true));
func SetRLSContext(ctx context.Context, tx pgx.Tx, claims *Claims) error {
	if claims == nil {
		return nil
	}

	// Switch to the authenticated role so RLS policies are enforced.
	_, err := tx.Exec(ctx, "SET LOCAL ROLE "+AuthenticatedRole)
	if err != nil {
		return fmt.Errorf("setting role: %w", err)
	}

	_, err = tx.Exec(ctx, "SELECT set_config('ayb.user_id', $1, true)", claims.Subject)
	if err != nil {
		return fmt.Errorf("setting ayb.user_id: %w", err)
	}

	_, err = tx.Exec(ctx, "SELECT set_config('ayb.user_email', $1, true)", claims.Email)
	if err != nil {
		return fmt.Errorf("setting ayb.user_email: %w", err)
	}

	return nil
}
