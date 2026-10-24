// Package targeterr maps errors from target databases to client-safe API errors.
package targeterr

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

// Map keeps *apperr.Error as is, turns timeouts and connection failures into
// TARGET_DB_TIMEOUT / TARGET_DB_UNAVAILABLE, and leaves the rest as internal errors.
func Map(err error) error {
	if err == nil {
		return nil
	}
	var ae *apperr.Error
	if errors.As(err, &ae) {
		return err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return apperr.ErrTargetDBTimeout
	}
	var connectErr *pgconn.ConnectError
	var netErr net.Error
	if errors.As(err, &connectErr) || errors.As(err, &netErr) || errors.Is(err, driver.ErrBadConn) {
		return apperr.ErrTargetDBUnavailable
	}
	return fmt.Errorf("target db: %w", err)
}
