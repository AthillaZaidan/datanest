package apperr

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestResponseAppError(t *testing.T) {
	status, body := Response(ErrForbidden)
	if status != http.StatusForbidden || body.Error.Code != "FORBIDDEN" {
		t.Fatalf("got %d %+v", status, body)
	}
}

func TestResponseWrappedAppError(t *testing.T) {
	err := fmt.Errorf("execute migration: %w", ErrMigrationRunning)
	status, body := Response(err)
	if status != http.StatusConflict || body.Error.Code != "MIGRATION_RUNNING" {
		t.Fatalf("got %d %+v", status, body)
	}
}

func TestResponseUnknownErrorHidesDetails(t *testing.T) {
	status, body := Response(errors.New("pq: password authentication failed for user secret"))
	if status != http.StatusInternalServerError || body.Error.Code != "INTERNAL_ERROR" {
		t.Fatalf("got %d %+v", status, body)
	}
	if body.Error.Message != ErrInternal.Message || body.Error.Details != nil {
		t.Fatalf("internal details leaked: %+v", body)
	}
}

func TestWithMessageAndDetailsCopy(t *testing.T) {
	details := map[string]string{"username": "wajib diisi"}
	err := ErrValidation.WithMessage("Username wajib diisi.").WithDetails(details)

	if ErrValidation.Message == err.Message || ErrValidation.Details != nil {
		t.Fatal("WithMessage/WithDetails must not modify the original error")
	}
	if !errors.Is(err, ErrValidation) {
		t.Fatal("copy should still match the original code with errors.Is")
	}
	if errors.Is(err, ErrForbidden) {
		t.Fatal("different codes must not match")
	}

	_, body := Response(err)
	if body.Error.Message != "Username wajib diisi." {
		t.Fatalf("message = %q", body.Error.Message)
	}
	if got, ok := body.Error.Details.(map[string]string); !ok || got["username"] != "wajib diisi" {
		t.Fatalf("details = %#v", body.Error.Details)
	}
}

func TestCodesAreUnique(t *testing.T) {
	all := []*Error{
		ErrValidation, ErrTransactionControlNotAllowed, ErrUnauthenticated, ErrInvalidCredentials,
		ErrForbidden, ErrCSRFInvalid, ErrOriginNotAllowed, ErrNotFound, ErrUsernameTaken, ErrConflict,
		ErrMigrationRunning, ErrMigrationNotDraft, ErrRevisionConflict, ErrUnsupportedMediaType,
		ErrInternal, ErrNotImplemented, ErrTargetDBUnavailable, ErrTargetDBTimeout,
	}
	seen := map[string]bool{}
	for _, e := range all {
		if seen[e.Code] {
			t.Errorf("duplicate code %s", e.Code)
		}
		seen[e.Code] = true
	}
}
