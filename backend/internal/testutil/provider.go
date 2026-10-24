package testutil

import (
	"context"
	"database/sql"
)

// StaticProvider is a dbconn.TargetDBProvider that returns the same pool for every database.
type StaticProvider struct {
	DB *sql.DB
}

func (p StaticProvider) Get(context.Context, uint) (*sql.DB, error) { return p.DB, nil }

func (p StaticProvider) Invalidate(uint) {}
