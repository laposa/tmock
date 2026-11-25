package httpx

import (
	"encoding/json"
	"errors"
	"maps"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type input struct {
	Name  string           `json:"name"`
	Count int              `json:"count"`
	Note  Optional[string] `json:"note"`
	Items []struct {
		Label string `json:"label"`
	} `json:"items"`
}

func decode(body string) (input, error) {
	var in input
	r := httptest.NewRequest("POST", "/", strings.NewReader(body))
	err := DecodeJSON(httptest.NewRecorder(), r, &in)
	return in, err
}

func TestDecodeJSON(t *testing.T) {
	t.Parallel()
	in, err := decode(`{"name":"a","count":2,"items":[{"label":"x"}]}`)
	if err != nil || in.Name != "a" || in.Count != 2 || len(in.Items) != 1 || in.Note.Set {
		t.Fatalf("%+v, %v", in, err)
	}
	if _, err := decode(" {} \n"); err != nil {
		t.Errorf("whitespace around the object: %v", err)
	}
}

func TestDecodeJSONErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name, body  string
		status      int
		message     string // prefix
		details     map[string]string
		ignoreField bool // the field is not reported by every Go version
	}{
		{"empty", ``, 400, "Request body is empty", nil, false},
		{"truncated", `{"name":`, 400, "Malformed JSON: unexpected end of input", nil, false},
		{"syntax", `{"name" "a"}`, 400, "Malformed JSON", nil, false},
		{"not an object", `[1,2]`, 400, "Request body must be a JSON object", nil, false},
		{"two objects", `{} {}`, 400, "Request body must contain a single JSON object", nil, false},
		{"trailing garbage", `{"name":"a"} x`, 400, "Request body must contain a single JSON object", nil, false},
		{"unknown field", `{"name":"a","nmae":"b"}`, 400, `Unknown field "nmae"`, map[string]string{"nmae": "unknown field"}, false},
		{"wrong type", `{"count":"two"}`, 400, "Validation failed", map[string]string{"count": "must be a whole number"}, false},
		{"wrong type nested", `{"items":[{"label":"ok"},{"label":7}]}`, 400, "Validation failed", map[string]string{"items[1].label": "must be a string"}, false},
		{"wrong type in Optional", `{"note":5}`, 400, "Validation failed", map[string]string{"note": "must be a string"}, true},
	}
	for _, tc := range cases {
		_, err := decode(tc.body)
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%s: %v (%T)", tc.name, err, err)
			continue
		}
		if e.Status != tc.status || !strings.HasPrefix(e.Message, tc.message) {
			t.Errorf("%s: %d %q", tc.name, e.Status, e.Message)
		}
		if tc.ignoreField && e.Details == nil {
			continue
		}
		if !maps.Equal(e.Details, tc.details) {
			t.Errorf("%s: details %v, want %v", tc.name, e.Details, tc.details)
		}
	}
}

func TestDecodeJSONTooLarge(t *testing.T) {
	t.Parallel()
	big := `{"name":"` + strings.Repeat("x", MaxBodyBytes) + `"}`
	_, err := decode(big)
	var e *Error
	if !errors.As(err, &e) || e.Status != http.StatusRequestEntityTooLarge || e.Message != "Request body is larger than 10 MB" {
		t.Errorf("got %v", err)
	}

	// Just under the limit is fine.
	ok := `{"name":"` + strings.Repeat("x", MaxBodyBytes-20) + `"}`
	if _, err := decode(ok); err != nil {
		t.Errorf("under the limit: %v", err)
	}

	// The limit also applies to data after the object.
	tail := `{}` + strings.Repeat(" ", MaxBodyBytes)
	if _, err := decode(tail); !errors.As(err, &e) || e.Status != http.StatusRequestEntityTooLarge {
		t.Errorf("large tail: %v", err)
	}
}

// Verify item: Optional[T] tells absent, null and a value apart.
func TestOptional(t *testing.T) {
	t.Parallel()
	for body, want := range map[string]Optional[string]{
		`{}`:            {},
		`{"note":null}`: Null[string](),
		`{"note":""}`:   Some(""),
		`{"note":"hi"}`: Some("hi"),
	} {
		in, err := decode(body)
		if err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if in.Note != want {
			t.Errorf("%s: %+v, want %+v", body, in.Note, want)
		}
	}

	if v, ok := Some(3).Get(); !ok || v != 3 {
		t.Error("Some.Get")
	}
	if _, ok := Null[int]().Get(); ok {
		t.Error("Null.Get has a value")
	}
	var absent Optional[int]
	if absent.Set || absent.HasValue() {
		t.Error("zero value is not absent")
	}

	// Nested and non-scalar types.
	var obj struct {
		Headers Optional[map[string]*string] `json:"headers"`
		IDs     Optional[[]int64]            `json:"ids"`
	}
	if err := json.Unmarshal([]byte(`{"headers":{"A":"1","B":null},"ids":[1,2]}`), &obj); err != nil {
		t.Fatal(err)
	}
	if !obj.Headers.HasValue() || *obj.Headers.Value["A"] != "1" || obj.Headers.Value["B"] != nil || len(obj.IDs.Value) != 2 {
		t.Errorf("%+v", obj)
	}
}

func TestFieldPath(t *testing.T) {
	t.Parallel()
	for in, want := range map[string]string{
		"name":                 "name",
		"items.2.label":        "items[2].label",
		"condition.and.1.or.0": "condition.and[1].or[0]",
		"responseHeaders.X-Id": "responseHeaders.X-Id",
	} {
		if got := FieldPath(in); got != want {
			t.Errorf("FieldPath(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestPathID(t *testing.T) {
	t.Parallel()
	mux := http.NewServeMux()
	var got int64
	var gotErr error
	mux.HandleFunc("GET /x/{id}", func(w http.ResponseWriter, r *http.Request) { got, gotErr = PathID(r, "id") })

	for path, want := range map[string]int64{"/x/1": 1, "/x/9007199254740993": 9007199254740993} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		if gotErr != nil || got != want {
			t.Errorf("%s: %d, %v", path, got, gotErr)
		}
	}
	for _, path := range []string{"/x/0", "/x/-1", "/x/abc", "/x/1.5", "/x/99999999999999999999"} {
		mux.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", path, nil))
		var e *Error
		if !errors.As(gotErr, &e) || e.Status != 400 || !strings.HasPrefix(e.Message, "id must be a positive whole number") {
			t.Errorf("%s: %v", path, gotErr)
		}
	}
}

func TestWriteJSON(t *testing.T) {
	t.Parallel()
	rec := httptest.NewRecorder()
	WriteJSON(rec, http.StatusCreated, map[string]any{"a": 1})
	if rec.Code != 201 || rec.Header().Get("Content-Type") != "application/json" || rec.Body.String() != `{"a":1}`+"\n" {
		t.Errorf("%d %v %q", rec.Code, rec.Header(), rec.Body.String())
	}
	rec = httptest.NewRecorder()
	NoContent(rec)
	if rec.Code != 204 || rec.Body.Len() != 0 {
		t.Errorf("NoContent: %d %q", rec.Code, rec.Body.String())
	}
}
