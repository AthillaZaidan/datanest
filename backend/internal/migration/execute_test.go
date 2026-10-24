package migration

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestFailure(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{"pg error with detail", &pgconn.PgError{Code: "23505", Message: "duplicate key", Detail: "Key (id)=(1) exists."}, "23505", "ERROR: duplicate key\nDETAIL: Key (id)=(1) exists."},
		{"not in transaction", fmt.Errorf("exec: %w", &pgconn.PgError{Code: "25001", Message: "cannot run inside a transaction block"}), "25001", "tidak dapat dijalankan dalam transaction"},
		{"timeout", fmt.Errorf("exec: %w", context.DeadlineExceeded), "", "batas waktu"},
		{"connection", errors.New("conn closed"), "", "Koneksi"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			msg, code := failure(tt.err)
			if code != tt.wantCode || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("failure = %q, %q; want code %q and message containing %q", msg, code, tt.wantCode, tt.wantMsg)
			}
		})
	}
}
