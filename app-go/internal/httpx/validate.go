package httpx

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Validator collects validation problems keyed by field path, for the details of a 400
// (spec §6.2) and for the messages next to UI form fields (§10.2). The first problem
// recorded for a field wins, so the most basic check (for example "is required") is the
// one reported.
//
//	var v httpx.Validator
//	v.Required("name", in.Name != "")
//	v.Length("name", in.Name, 1, 255)
//	if !v.OK() {
//		return app.Invalid(v.Details())
//	}
type Validator struct {
	details map[string]string
}

// Add records msg for field, unless the field already has a problem.
func (v *Validator) Add(field, msg string) {
	if v.details == nil {
		v.details = map[string]string{}
	}
	if _, ok := v.details[field]; !ok {
		v.details[field] = msg
	}
}

// Check records msg for field when ok is false. It returns ok, so checks can be chained.
func (v *Validator) Check(ok bool, field, msg string) bool {
	if !ok {
		v.Add(field, msg)
	}
	return ok
}

// Required records "is required" when present is false.
func (v *Validator) Required(field string, present bool) bool {
	return v.Check(present, field, "is required")
}

// Length checks that s has between min and max characters (not bytes).
func (v *Validator) Length(field, s string, min, max int) bool {
	n := utf8.RuneCountInString(s)
	return v.Check(n >= min && n <= max, field, fmt.Sprintf("must be %d–%d characters", min, max))
}

// Has reports whether field already has a problem.
func (v *Validator) Has(field string) bool {
	_, ok := v.details[field]
	return ok
}

// OK reports whether no problem was recorded.
func (v *Validator) OK() bool { return len(v.details) == 0 }

// Details returns the problems, or nil if there are none.
func (v *Validator) Details() map[string]string { return v.details }

// Field builds a field path in the notation of spec §6.2: strings are names joined with
// dots, ints are indices in brackets. Field("condition", "and", 1, "ip") is
// "condition.and[1].ip".
func Field(parts ...any) string {
	var b strings.Builder
	for _, p := range parts {
		switch p := p.(type) {
		case int:
			b.WriteByte('[')
			b.WriteString(strconv.Itoa(p))
			b.WriteByte(']')
		case string:
			if b.Len() > 0 {
				b.WriteByte('.')
			}
			b.WriteString(p)
		default:
			panic(fmt.Sprintf("httpx.Field: unsupported part %T", p))
		}
	}
	return b.String()
}
