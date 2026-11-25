package httpx

import (
	"maps"
	"strings"
	"testing"
)

func TestValidator(t *testing.T) {
	t.Parallel()
	var v Validator
	if !v.OK() || v.Details() != nil {
		t.Fatal("a new Validator must be OK")
	}

	if v.Required("name", false) {
		t.Error("Required(false) returned true")
	}
	v.Length("name", "", 1, 255) // second problem for the same field: ignored
	v.Length("path", strings.Repeat("ü", 256), 1, 255)
	v.Length("ok", strings.Repeat("ü", 255), 1, 255) // 255 characters, 510 bytes: fine
	v.Check(false, Field("condition", "and", 1, "ip"), "must be an IP address")
	v.Check(true, "never", "not recorded")

	want := map[string]string{
		"name":                "is required",
		"path":                "must be 1–255 characters",
		"condition.and[1].ip": "must be an IP address",
	}
	if v.OK() || !maps.Equal(v.Details(), want) {
		t.Errorf("details %v", v.Details())
	}
	if !v.Has("path") || v.Has("ok") {
		t.Error("Has")
	}
}

func TestField(t *testing.T) {
	t.Parallel()
	for want, parts := range map[string][]any{
		"name":                 {"name"},
		"condition.or[0].cidr": {"condition", "or", 0, "cidr"},
		"[2]":                  {2},
		"scenarios[3]":         {"scenarios", 3},
		"a.b[0][1].c":          {"a", "b", 0, 1, "c"},
	} {
		if got := Field(parts...); got != want {
			t.Errorf("Field(%v) = %q, want %q", parts, got, want)
		}
	}
}
