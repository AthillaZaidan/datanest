package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
)

func TestDescribe(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
		wantMsg  string
	}{
		{"pg error with detail", &pgconn.PgError{Code: "23505", Message: "duplicate key", Detail: "Key (id)=(1) exists."}, "23505", "ERROR: duplicate key\nDETAIL: Key (id)=(1) exists."},
		{"pg error with hint", &pgconn.PgError{Code: "42703", Message: "no column", Hint: "Perhaps you meant x."}, "42703", "ERROR: no column\nHINT: Perhaps you meant x."},
		{"not in transaction", fmt.Errorf("exec: %w", &pgconn.PgError{Code: "25001", Message: "cannot run inside a transaction block"}), "25001", "tidak dapat dijalankan dalam transaction"},
		{"timeout", fmt.Errorf("exec: %w", context.DeadlineExceeded), "TIMEOUT", "batas waktu"},
		{"connection", errors.New("conn closed"), "TARGET_UNAVAILABLE", "Koneksi"},
		{"uncertain commit", &unknownError{err: errors.New("eof")}, "COMMIT_UNKNOWN", "commit tidak pasti"},
		{"permanent", permanent("SQL_CHANGED", "berubah %d", 1), "SQL_CHANGED", "berubah 1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, msg := describe(tt.err)
			if code != tt.wantCode || !strings.Contains(msg, tt.wantMsg) {
				t.Fatalf("describe = %q, %q; want code %q and message containing %q", code, msg, tt.wantCode, tt.wantMsg)
			}
		})
	}
}
