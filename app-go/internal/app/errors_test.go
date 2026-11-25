package app

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/httpx"
	"github.com/laposa/tmock/app-go/internal/store"
)

func TestErrorStatus(t *testing.T) {
	t.Parallel()
	for err, want := range map[*Error]int{
		Invalid(map[string]string{"name": "is required"}):        400,
		InvalidField("email", "must be an email address"):        400,
		BadRequest("You cannot delete your own account"):         400,
		Unauthorized("Wrong email or password"):                  401,
		Forbidden("Admins only"):                                 403,
		NotFound("Service %q not found", "svc"):                  404,
		Conflict("A service with path %q already exists", "svc"): 409,
		{Message: "no kind"}:                                     500,
	} {
		if got := err.StatusCode(); got != want {
			t.Errorf("%q: %d, want %d", err.Message, got, want)
		}
	}
	if NotFound("Service %q not found", "svc").Message != `Service "svc" not found` {
		t.Error("message formatting")
	}
}

// The convention shared by the API and the UI: an *Error, even wrapped, is written as the
// §6.2 body; anything else is a 500 that hides the details.
func TestErrorsAreWrittenByHTTPX(t *testing.T) {
	t.Parallel()
	var logs bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&logs, nil))

	cases := []struct {
		err  error
		want string
	}{
		{InvalidField("condition.or[0].cidr", "must be a CIDR prefix"), `{"status":400,"message":"Validation failed","details":{"condition.or[0].cidr":"must be a CIDR prefix"}}`},
		{fmt.Errorf("creating service: %w", Conflict("A service with path %q already exists", "svc")), `{"status":409,"message":"A service with path \"svc\" already exists"}`},
		{errors.New("pq: connection reset with password=secret"), `{"status":500,"message":"Internal server error"}`},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		httpx.WriteErr(rec, httptest.NewRequest("POST", "/api/service", nil), logger, tc.err)
		if got := strings.TrimSpace(rec.Body.String()); got != tc.want {
			t.Errorf("%v: %s", tc.err, got)
		}
	}
	if !strings.Contains(logs.String(), "connection reset") || strings.Count(logs.String(), "\n") != 1 {
		t.Errorf("only the unexpected error must be logged:\n%s", logs.String())
	}
}

func TestStoreError(t *testing.T) {
	t.Parallel()
	nf := func() *Error { return NotFound("User %d not found", 7) }
	cf := func() *Error { return Conflict("A user with this email already exists") }

	if StoreError(nil, nf(), cf()) != nil {
		t.Error("nil")
	}
	err := StoreError(store.ErrNotFound, nf(), cf())
	if KindOf(err) != KindNotFound || !errors.Is(err, store.ErrNotFound) {
		t.Errorf("not found: %v", err)
	}
	err = StoreError(fmt.Errorf("insert: %w", store.ErrUnique), nf(), cf())
	if KindOf(err) != KindConflict || !errors.Is(err, store.ErrUnique) {
		t.Errorf("unique: %v", err)
	}
	if err := StoreError(store.ErrForeignKey, nil, nil); KindOf(err) != KindInvalid {
		t.Errorf("foreign key: %v", err)
	}
	// Without a matching translation the error stays unexpected.
	if err := StoreError(store.ErrNotFound, nil, cf()); KindOf(err) != 0 {
		t.Errorf("unmapped not found: %v", err)
	}
	shared := NotFound("Shared")
	if StoreError(store.ErrNotFound, shared, nil) == error(shared) || shared.Err != nil {
		t.Error("StoreError changed the error it was given")
	}
	other := errors.New("boom")
	if StoreError(other, nf(), cf()) != other {
		t.Error("other errors must pass through")
	}

	e := Invalid(map[string]string{"a": "b"})
	if !maps.Equal(e.FieldDetails(), map[string]string{"a": "b"}) || e.PublicMessage() != "Validation failed" {
		t.Errorf("%+v", e)
	}
}
