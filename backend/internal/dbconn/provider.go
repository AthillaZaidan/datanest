// Package dbconn manages connections to target databases.
package dbconn

import (
	"context"
	"database/sql"
)

// TargetDBProvider hands out shared pools to target databases.
type TargetDBProvider interface {
	// Get returns a shared pool; callers must not close it.
	Get(ctx context.Context, databaseID uint) (*sql.DB, error)
	// Invalidate drops the pool after the connection is edited or deleted.
	Invalidate(databaseID uint)
}
