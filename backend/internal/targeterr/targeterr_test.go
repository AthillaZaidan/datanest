package targeterr

import (
	"context"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/AthillaZaidan/datanest/backend/internal/apperr"
)

func TestMap(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want *apperr.Error
	}{
		{"app error kept", apperr.ErrForbidden, apperr.ErrForbidden},
		{"deadline", fmt.Errorf("query: %w", context.DeadlineExceeded), apperr.ErrTargetDBTimeout},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("connection refused")}, apperr.ErrTargetDBUnavailable},
		{"bad conn", driver.ErrBadConn, apperr.ErrTargetDBUnavailable},
		{"other", errors.New("syntax error"), apperr.ErrInternal},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, body := apperr.Response(Map(tt.err))
			if body.Error.Code != tt.want.Code {
				t.Fatalf("code = %s, want %s", body.Error.Code, tt.want.Code)
			}
		})
	}
	if Map(nil) != nil {
		t.Fatal("Map(nil) should be nil")
	}
}
