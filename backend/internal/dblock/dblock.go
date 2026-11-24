package dblock

import (
	"context"
	"time"

	"gorm.io/gorm"
)

type Lease struct {
	DatabaseID uint
	Owner      string
	Token      int64
}

type Store struct {
	db *gorm.DB
}

func New(db *gorm.DB) *Store {
	return &Store{db: db}
}

func (s *Store) Acquire(ctx context.Context, databaseID, executionID uint, owner string, ttl time.Duration) (Lease, bool, error) {
	var tokens []int64
	err := s.db.WithContext(ctx).Raw(`
		INSERT INTO database_locks (database_id, execution_id, owner, token, lease_until)
		VALUES (?, ?, ?, 1, now() + make_interval(secs => ?::float8))
		ON CONFLICT (database_id) DO UPDATE SET
			execution_id = EXCLUDED.execution_id,
			owner = EXCLUDED.owner,
			token = database_locks.token + 1,
			lease_until = EXCLUDED.lease_until
		WHERE database_locks.lease_until < now()
		RETURNING token`,
		databaseID, executionID, owner, ttl.Seconds(),
	).Scan(&tokens).Error
	if err != nil {
		return Lease{}, false, err
	}
	if len(tokens) == 0 {
		return Lease{}, false, nil
	}
	return Lease{DatabaseID: databaseID, Owner: owner, Token: tokens[0]}, true, nil
}

func (s *Store) Renew(ctx context.Context, l Lease, ttl time.Duration) (bool, error) {
	res := s.db.WithContext(ctx).Exec(`
		UPDATE database_locks SET lease_until = now() + make_interval(secs => ?::float8)
		WHERE database_id = ? AND owner = ? AND token = ?`,
		ttl.Seconds(), l.DatabaseID, l.Owner, l.Token,
	)
	return res.RowsAffected == 1, res.Error
}

func (s *Store) Release(ctx context.Context, l Lease) error {
	return s.db.WithContext(ctx).Exec(`
		UPDATE database_locks SET lease_until = now() - interval '1 second'
		WHERE database_id = ? AND owner = ? AND token = ?`,
		l.DatabaseID, l.Owner, l.Token,
	).Error
}
