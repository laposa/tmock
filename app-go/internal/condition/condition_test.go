package condition

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func mustParse(t *testing.T, s string) *Condition {
	t.Helper()
	c, err := Parse(s)
	if err != nil {
		t.Fatalf("Parse(%s): %v", s, err)
	}
	return c
}

// JSON round-trips in the shape of spec §6.1; an empty and/or list is kept as [].
func TestJSONRoundTrip(t *testing.T) {
	t.Parallel()
	for _, s := range []string{
		`{"and":[]}`,
		`{"or":[]}`,
		`{"ip":"10.0.0.1"}`,
		`{"not":true,"cidr":"10.0.0.0/8"}`,
		`{"and":[{"headerMatch":{"header":"x-test","value":"1"}},{"or":[{"headerRegex":{"header":"user-agent","value":"^curl/"}},{"not":true,"ip":"::1"}]}]}`,
	} {
		c := mustParse(t, s)
		b, err := json.Marshal(c)
		if err != nil || string(b) != s {
			t.Errorf("round trip of %s: %s, %v", s, b, err)
		}
	}
}

func TestParse(t *testing.T) {
	t.Parallel()
	for _, s := range []string{"", "  ", "null", " null "} {
		if c, err := Parse(s); c != nil || err != nil {
			t.Errorf("Parse(%q) = %v, %v; want no condition", s, c, err)
		}
	}
	for s, want := range map[string]string{
		`{"ip":`:                            "the JSON ends too early",
		`{"ip":1}`:                          "ip has the wrong type",
		`{"ipp":"1.2.3.4"}`:                 `unknown field "ipp"`,
		`{"and":[{"cidrr":"x"}]}`:           `unknown field "cidrr"`,
		`{"headerMatch":{"name":"x"}}`:      `unknown field "name"`,
		`{"ip":"1.2.3.4"} {"ip":"5.6.7.8"}`: "unexpected text after the condition",
		`{"ip" "x"}`:                        "invalid JSON at character",
		`[]`:                                "wrong type",
	} {
		_, err := Parse(s)
		if !errors.Is(err, ErrSyntax) || !strings.Contains(err.Error(), want) {
			t.Errorf("Parse(%s) = %v; want %q", s, err, want)
		}
	}
}

// Verify item: nested condition validation with field paths (spec §6.4).
func TestValidate(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("x", 256)
	cases := []struct {
		cond string
		want []Problem
	}{
		{`{"ip":"10.0.0.1"}`, nil},
		{`{"ip":"2001:db8::1"}`, nil},
		{`{"cidr":"2001:db8::/32"}`, nil},
		{`{"and":[]}`, nil},
		{`{"headerRegex":{"header":"x","value":"^a+$"}}`, nil},
		{`{}`, []Problem{{"", "must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has 0)"}}},
		{`{"not":true}`, []Problem{{"", "must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has 0)"}}},
		{`{"ip":"1.2.3.4","cidr":"1.0.0.0/8"}`, []Problem{{"", "must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has 2)"}}},
		{`{"ip":"999.1.1.1"}`, []Problem{{"ip", "must be an IPv4 or IPv6 address"}}},
		{`{"ip":"fe80::1%eth0"}`, []Problem{{"ip", "must be an IPv4 or IPv6 address"}}},
		{`{"cidr":"10.0.0.0"}`, []Problem{{"cidr", "must be a CIDR prefix, such as 10.0.0.0/8 or 2001:db8::/32"}}},
		{`{"or":[{"ip":"1.2.3.4"},{"cidr":"x/99"}]}`, []Problem{{"or[1].cidr", "must be a CIDR prefix, such as 10.0.0.0/8 or 2001:db8::/32"}}},
		{`{"and":[{"or":[{"headerMatch":{"header":"","value":"` + long + `"}}]}]}`, []Problem{
			{"and[0].or[0].headerMatch.header", "must be 1–255 characters"},
			{"and[0].or[0].headerMatch.value", "must be 1–255 characters"},
		}},
		{`{"and":[{"headerRegex":{"header":"x","value":"(a"}}, {}]}`, []Problem{
			{"and[0].headerRegex.value", "is not a valid regular expression: missing closing ): `(a`"},
			{"and[1]", "must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has 0)"},
		}},
		// RE2: no backreferences.
		{`{"headerRegex":{"header":"x","value":"(a)\\1"}}`, []Problem{{"headerRegex.value", "is not a valid regular expression: invalid escape sequence: `\\1`"}}},
	}
	for _, tc := range cases {
		got := Validate(mustParse(t, tc.cond))
		if len(got) != len(tc.want) {
			t.Errorf("Validate(%s) = %v, want %v", tc.cond, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("Validate(%s)[%d] = %v, want %v", tc.cond, i, got[i], tc.want[i])
			}
		}
	}
	if p := Validate(nil); p != nil {
		t.Errorf("Validate(nil) = %v", p)
	}
}

// Spec §7.3: an empty condition is null or has no ip, cidr, headerMatch or headerRegex.
func TestEmpty(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{
		`null`:                           true,
		`{"and":[]}`:                     true,
		`{"or":[{"and":[]}, {"or":[]}]}`: true,
		`{"not":true,"and":[]}`:          true,
		`{"ip":"1.2.3.4"}`:               false,
		`{"and":[{"or":[{"cidr":"10.0.0.0/8"}]}]}`:              false,
		`{"not":true,"headerMatch":{"header":"a","value":"b"}}`: false,
	} {
		if got := Empty(mustParse(t, s)); got != want {
			t.Errorf("Empty(%s) = %v, want %v", s, got, want)
		}
	}
}

// Parsing and validating never panic (spec §11.1 fuzz level).
func FuzzParseValidate(f *testing.F) {
	for _, s := range []string{`{"and":[]}`, `{"or":[{"ip":"1.2.3.4"}]}`, `{"headerRegex":{"header":"a","value":"(b"}}`, `null`, `{`} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		c, err := Parse(s)
		if err != nil {
			return
		}
		Validate(c)
		Empty(c)
		if _, err := json.Marshal(c); err != nil {
			t.Fatalf("marshal: %v", err)
		}
	})
}
