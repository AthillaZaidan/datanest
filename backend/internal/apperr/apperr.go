// Package apperr defines the application's error type and the standard error codes of the API.
//
// Services return *Error values; the HTTP layer turns any error into a JSON body with Response.
// Errors that are not *Error become INTERNAL_ERROR, so internal details never reach the client.
package apperr

import (
	"errors"
	"net/http"
)

// Error is an error that is safe to show to API clients.
type Error struct {
	Status  int
	Code    string
	Message string
	Details any
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

// Is reports whether target has the same code, so errors.Is(err, apperr.ErrForbidden) works
// for copies made with WithMessage or WithDetails.
func (e *Error) Is(target error) bool {
	var t *Error
	return errors.As(target, &t) && t.Code == e.Code
}

// WithMessage returns a copy of e with a different message.
func (e *Error) WithMessage(msg string) *Error {
	c := *e
	c.Message = msg
	return &c
}

// WithDetails returns a copy of e carrying details (for example per-field validation errors).
func (e *Error) WithDetails(details any) *Error {
	c := *e
	c.Details = details
	return &c
}

// New creates an error with the given HTTP status, code, and message.
func New(status int, code, msg string) *Error {
	return &Error{Status: status, Code: code, Message: msg}
}

// Standard errors. Use WithMessage / WithDetails to customise them.
var (
	ErrValidation                   = New(http.StatusBadRequest, "VALIDATION_ERROR", "Input tidak valid.")
	ErrTransactionControlNotAllowed = New(http.StatusBadRequest, "TRANSACTION_CONTROL_NOT_ALLOWED", "SQL migration tidak boleh berisi perintah kontrol transaction (BEGIN, COMMIT, ROLLBACK, dan sejenisnya).")
	ErrUnauthenticated              = New(http.StatusUnauthorized, "UNAUTHENTICATED", "Silakan login terlebih dahulu.")
	ErrInvalidCredentials           = New(http.StatusUnauthorized, "INVALID_CREDENTIALS", "Username atau password salah.")
	ErrForbidden                    = New(http.StatusForbidden, "FORBIDDEN", "Anda tidak memiliki akses.")
	ErrCSRFInvalid                  = New(http.StatusForbidden, "CSRF_INVALID", "Token CSRF tidak valid.")
	ErrOriginNotAllowed             = New(http.StatusForbidden, "ORIGIN_NOT_ALLOWED", "Origin request tidak diizinkan.")
	ErrNotFound                     = New(http.StatusNotFound, "NOT_FOUND", "Resource tidak ditemukan.")
	ErrUsernameTaken                = New(http.StatusConflict, "USERNAME_TAKEN", "Username sudah dipakai.")
	ErrConflict                     = New(http.StatusConflict, "CONFLICT", "Data bentrok dengan data yang sudah ada.")
	ErrMigrationRunning             = New(http.StatusConflict, "MIGRATION_RUNNING", "Sedang ada migration yang berjalan di database ini.")
	ErrMigrationNotDraft            = New(http.StatusConflict, "MIGRATION_NOT_DRAFT", "Migration ini bukan draft.")
	ErrRevisionConflict             = New(http.StatusConflict, "REVISION_CONFLICT", "Draft telah diubah oleh pengguna lain. Muat ulang untuk melihat perubahan terbaru.")
	ErrUnsupportedMediaType         = New(http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "Content-Type harus application/json.")
	ErrInternal                     = New(http.StatusInternalServerError, "INTERNAL_ERROR", "Terjadi kesalahan pada server.")
	ErrNotImplemented               = New(http.StatusNotImplemented, "NOT_IMPLEMENTED", "Fitur ini belum diimplementasikan.")
	ErrTargetDBUnavailable          = New(http.StatusBadGateway, "TARGET_DB_UNAVAILABLE", "Gagal terhubung ke database target.")
	ErrTargetDBTimeout              = New(http.StatusGatewayTimeout, "TARGET_DB_TIMEOUT", "Database target tidak merespons tepat waktu.")
)

// Body is the JSON shape of every error response.
type Body struct {
	Error BodyError `json:"error"`
}

// BodyError is the inner object of Body.
type BodyError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details any    `json:"details,omitempty"`
}

// Response maps err to an HTTP status and response body.
func Response(err error) (int, Body) {
	var e *Error
	if !errors.As(err, &e) {
		e = ErrInternal
	}
	return e.Status, Body{Error: BodyError{Code: e.Code, Message: e.Message, Details: e.Details}}
}
