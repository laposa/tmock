package app

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/store"
)

// Kind classifies a use-case error. The REST API and the UI both map it to an HTTP status
// (spec §6.2, §10.2), through Error.StatusCode.
type Kind int

const (
	KindInvalid      Kind = iota + 1 // validation failed; Details names the fields → 400
	KindUnauthorized                 // not logged in, bad credentials, unknown token → 401
	KindForbidden                    // logged in, but not allowed → 403
	KindNotFound                     // → 404
	KindConflict                     // duplicate path, name or email → 409
)

// Error is the typed error every use case returns for an expected failure. Its Message
// and Details are safe to show to the user; the REST API writes them as the §6.2 body,
// and the UI shows them as a toast and next to the form fields. Any other error a use
// case returns is unexpected: it becomes a 500 and is logged, never shown.
type Error struct {
	Kind    Kind
	Message string
	Details map[string]string // field path → message; only for KindInvalid
	Err     error             // the underlying cause, if any; never shown
}

func (e *Error) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

func (e *Error) Unwrap() error { return e.Err }

// StatusCode is the HTTP status for the error's kind.
func (e *Error) StatusCode() int {
	switch e.Kind {
	case KindInvalid:
		return http.StatusBadRequest
	case KindUnauthorized:
		return http.StatusUnauthorized
	case KindForbidden:
		return http.StatusForbidden
	case KindNotFound:
		return http.StatusNotFound
	case KindConflict:
		return http.StatusConflict
	}
	return http.StatusInternalServerError
}

// PublicMessage and FieldDetails let internal/httpx write the error (httpx.PublicError).
func (e *Error) PublicMessage() string           { return e.Message }
func (e *Error) FieldDetails() map[string]string { return e.Details }

// Invalid is a validation error with per-field messages, as collected by httpx.Validator.
func Invalid(details map[string]string) *Error {
	return &Error{Kind: KindInvalid, Message: "Validation failed", Details: details}
}

// InvalidField is a validation error for a single field.
func InvalidField(field, msg string) *Error {
	return Invalid(map[string]string{field: msg})
}

// BadRequest is a 400 that is not about one field, such as deleting one's own account.
func BadRequest(format string, args ...any) *Error {
	return &Error{Kind: KindInvalid, Message: fmt.Sprintf(format, args...)}
}

// Unauthorized is a 401.
func Unauthorized(format string, args ...any) *Error {
	return &Error{Kind: KindUnauthorized, Message: fmt.Sprintf(format, args...)}
}

// Forbidden is a 403.
func Forbidden(format string, args ...any) *Error {
	return &Error{Kind: KindForbidden, Message: fmt.Sprintf(format, args...)}
}

// NotFound is a 404.
func NotFound(format string, args ...any) *Error {
	return &Error{Kind: KindNotFound, Message: fmt.Sprintf(format, args...)}
}

// Conflict is a 409.
func Conflict(format string, args ...any) *Error {
	return &Error{Kind: KindConflict, Message: fmt.Sprintf(format, args...)}
}

// StoreError translates the store's errors (spec §5.3) for one use case: ErrNotFound and
// ErrUnique become notFound and conflict (either may be nil when the case cannot happen),
// ErrForeignKey a 400. Other errors are returned unchanged, so they end up as a 500.
// notFound and conflict are copied, never changed, so they may be shared values.
//
//	err = app.StoreError(err, app.NotFound("Service %q not found", path),
//		app.Conflict("A service with path %q already exists", in.Path))
func StoreError(err error, notFound, conflict *Error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, store.ErrNotFound) && notFound != nil:
		return notFound.wrap(err)
	case errors.Is(err, store.ErrUnique) && conflict != nil:
		return conflict.wrap(err)
	case errors.Is(err, store.ErrForeignKey):
		return &Error{Kind: KindInvalid, Message: "A referenced item does not exist", Err: err}
	}
	return err
}

// wrap returns a copy of e with cause err.
func (e *Error) wrap(err error) *Error {
	c := *e
	c.Err = err
	return &c
}

// Kind of err, if it is (or wraps) an *Error; 0 otherwise.
func KindOf(err error) Kind {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return 0
}
