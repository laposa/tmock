package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"reflect"
	"regexp"
	"strconv"
	"strings"
)

// MaxBodyBytes is the request body limit (spec §6).
const MaxBodyBytes = 10 << 20

// DecodeJSON reads the request body, which must be one JSON object, into dst (a pointer
// to a struct). Unknown fields are rejected (spec §6). The errors are *Error values ready
// to be written with WriteErr:
//
//   - 413 when the body is larger than MaxBodyBytes;
//   - 400 for an empty body, malformed JSON, anything after the object, an unknown field
//     (details: {"field": "unknown field"}), or a value of the wrong type (details keyed
//     by the field path, for example "items[2].name").
func DecodeJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	r.Body = http.MaxBytesReader(w, r.Body, MaxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return decodeError(err, dst)
	}
	if err := dec.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return decodeError(err, dst)
		}
		return NewError(http.StatusBadRequest, "Request body must contain a single JSON object")
	}
	return nil
}

var unknownField = regexp.MustCompile(`^json: unknown field "(.*)"$`)

func decodeError(err error, dst any) error {
	var (
		tooLarge *http.MaxBytesError
		syntax   *json.SyntaxError
		typ      *json.UnmarshalTypeError
	)
	switch {
	case errors.As(err, &tooLarge):
		return NewError(http.StatusRequestEntityTooLarge, fmt.Sprintf("Request body is larger than %d MB", MaxBodyBytes>>20))
	case errors.Is(err, io.EOF):
		return NewError(http.StatusBadRequest, "Request body is empty; expected a JSON object")
	case errors.Is(err, io.ErrUnexpectedEOF):
		return NewError(http.StatusBadRequest, "Malformed JSON: unexpected end of input")
	case errors.As(err, &syntax):
		return NewError(http.StatusBadRequest, fmt.Sprintf("Malformed JSON at byte %d: %s", syntax.Offset, strings.TrimPrefix(syntax.Error(), "json: ")))
	case errors.As(err, &typ):
		if typ.Field == "" && dst != nil && typ.Type == reflect.TypeOf(dst).Elem() {
			return NewError(http.StatusBadRequest, "Request body must be a JSON object")
		}
		msg := "must be " + typeName(typ.Type)
		if typ.Field == "" {
			// Go 1.27's encoding/json does not report the field for types with their own
			// UnmarshalJSON, such as Optional[T].
			return &Error{Status: http.StatusBadRequest, Message: "Validation failed: a value " + msg + ", not " + typ.Value}
		}
		return &Error{Status: http.StatusBadRequest, Message: "Validation failed", Details: map[string]string{FieldPath(typ.Field): msg}}
	}
	if m := unknownField.FindStringSubmatch(err.Error()); m != nil {
		return &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("Unknown field %q", m[1]), Details: map[string]string{m[1]: "unknown field"}}
	}
	return NewError(http.StatusBadRequest, "Malformed JSON: "+strings.TrimPrefix(err.Error(), "json: "))
}

// FieldPath turns encoding/json's dotted path ("items.2.name") into the notation of
// spec §6.2 ("items[2].name").
func FieldPath(dotted string) string {
	var b strings.Builder
	for i, part := range strings.Split(dotted, ".") {
		if _, err := strconv.Atoi(part); err == nil && i > 0 {
			b.WriteByte('[')
			b.WriteString(part)
			b.WriteByte(']')
			continue
		}
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(part)
	}
	return b.String()
}

// typeName describes a Go type in JSON terms for an error message.
func typeName(t reflect.Type) string {
	for t.Kind() == reflect.Pointer {
		t = t.Elem()
	}
	switch t.Kind() {
	case reflect.String:
		return "a string"
	case reflect.Bool:
		return "true or false"
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		return "a whole number"
	case reflect.Float32, reflect.Float64:
		return "a number"
	case reflect.Slice, reflect.Array:
		return "an array"
	case reflect.Map, reflect.Struct:
		return "an object"
	}
	return "a " + t.String()
}

// WriteJSON writes v as a JSON response with the given status.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("httpx.WriteJSON: %v", err)) // a programming error; Recover answers 500
	}
	h := w.Header()
	h.Set("Content-Type", "application/json")
	h.Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = w.Write(append(b, '\n'))
}

// NoContent writes a 204 response.
func NoContent(w http.ResponseWriter) { w.WriteHeader(http.StatusNoContent) }

// PathID parses the path value name as a positive integer id. Anything else is a 400
// (spec §6.2: "non-integer id").
func PathID(r *http.Request, name string) (int64, error) {
	raw := r.PathValue(name)
	id, err := strconv.ParseInt(raw, 10, 64)
	if err != nil || id <= 0 {
		return 0, &Error{Status: http.StatusBadRequest, Message: fmt.Sprintf("%s must be a positive whole number, got %q", name, raw)}
	}
	return id, nil
}
