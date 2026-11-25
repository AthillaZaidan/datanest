package worker

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
)

type failureKind int

const (
	kindPermanent failureKind = iota
	kindTransient
	kindUnknown
)

type unknownError struct{ err error }

func (e *unknownError) Error() string { return "hasil commit tidak pasti: " + e.err.Error() }
func (e *unknownError) Unwrap() error { return e.err }

type permanentError struct {
	code string
	msg  string
}

func (e *permanentError) Error() string { return e.msg }

func permanent(code, format string, args ...any) error {
	return &permanentError{code: code, msg: fmt.Sprintf(format, args...)}
}

var transientCodes = map[string]bool{
	"55P03": true,
	"40P01": true,
	"40001": true,
	"57P01": true,
	"57P02": true,
	"57P03": true,
	"53300": true,
}

func classify(err error) failureKind {
	var unknown *unknownError
	var perm *permanentError
	switch {
	case errors.As(err, &unknown):
		return kindUnknown
	case errors.As(err, &perm):
		return kindPermanent
	case errors.Is(err, ErrInjectedTransient):
		return kindTransient
	case errors.Is(err, context.Canceled):
		return kindUnknown
	case errors.Is(err, context.DeadlineExceeded):
		return kindPermanent
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if transientCodes[pg.Code] || strings.HasPrefix(pg.Code, "08") {
			return kindTransient
		}
		return kindPermanent
	}
	return kindTransient
}

const msgNotInTransaction = "Statement ini tidak dapat dijalankan dalam transaction (misal CREATE INDEX CONCURRENTLY, VACUUM). " +
	"Platform menjalankan migration dalam transaction agar dapat di-rollback."

func describe(err error) (code, message string) {
	var perm *permanentError
	if errors.As(err, &perm) {
		return perm.code, perm.msg
	}
	var pg *pgconn.PgError
	if errors.As(err, &pg) {
		if pg.Code == "25001" {
			return pg.Code, msgNotInTransaction
		}
		message = "ERROR: " + pg.Message
		if pg.Detail != "" {
			message += "\nDETAIL: " + pg.Detail
		}
		if pg.Hint != "" {
			message += "\nHINT: " + pg.Hint
		}
		return pg.Code, message
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "TIMEOUT", "Eksekusi melebihi batas waktu; perubahan di-rollback."
	}
	var unknown *unknownError
	if errors.As(err, &unknown) {
		return "COMMIT_UNKNOWN", "Hasil commit tidak pasti; platform memeriksa ledger sebelum menentukan hasilnya."
	}
	return "TARGET_UNAVAILABLE", "Koneksi ke database target bermasalah."
}

func withJitter(d time.Duration) time.Duration {
	if d <= 0 {
		return d
	}
	return d + time.Duration(rand.Int64N(int64(d)/5+1))
}
