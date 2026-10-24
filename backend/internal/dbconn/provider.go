// Package dbconn manages connections to target databases.
package dbconn

import (
	"context"
	"database/sql"
)

// TargetDBProvider hands out connection pools to target databases.
//
// It is the contract between the connection module (owned by B) and its users such as the
// explorer and migration modules (owned by C). Until the real pool-backed implementation exists,
// tests can use a provider that opens the test target database directly.
type TargetDBProvider interface {
	// Get returns the pool for the database connection with the given ID.
	// The pool is shared and must not be closed by the caller.
	Get(ctx context.Context, databaseID uint) (*sql.DB, error)

	// Invalidate closes and forgets the pool of a connection that was edited or deleted.
	Invalidate(databaseID uint)
}
