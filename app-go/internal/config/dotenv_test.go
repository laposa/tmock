package config

import (
	"maps"
	"strings"
	"testing"
)

func TestParseDotenv(t *testing.T) {
	in := "\uFEFF# comment\n" +
		"\n" +
		"PORT=3000\n" +
		"  SPACED  =  value with spaces  \n" +
		"EMPTY=\n" +
		"INLINE=abc # trailing comment\n" +
		"HASH=abc#def\n" +
		"SINGLE='lit # eral \\n'\n" +
		`DOUBLE="a \"q\" \\ b\nc" # comment` + "\n" +
		"URL=postgres://u:p@h/db?sslmode=require\n" +
		"DUP=first\n" +
		"DUP=second\n"
	got, err := ParseDotenv(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"PORT":   "3000",
		"SPACED": "value with spaces",
		"EMPTY":  "",
		"INLINE": "abc",
		"HASH":   "abc#def",
		"SINGLE": `lit # eral \n`,
		"DOUBLE": "a \"q\" \\ b\nc",
		"URL":    "postgres://u:p@h/db?sslmode=require",
		"DUP":    "second",
	}
	if !maps.Equal(got, want) {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

// The legacy api/.env.example must parse, so existing files can be reused.
func TestParseDotenvLegacyExample(t *testing.T) {
	in := "TZ=Europe/Dublin\nDATABASE_HOST=\nDATABASE_ENABLE_SSL=true/false\n" +
		"## Set to true if tmock is behind a reverse proxy\nSERVICES_UPSTREAM_OVERRIDES=\n"
	got, err := ParseDotenv(strings.NewReader(in))
	if err != nil {
		t.Fatal(err)
	}
	if got["TZ"] != "Europe/Dublin" || got["DATABASE_ENABLE_SSL"] != "true/false" {
		t.Errorf("got %q", got)
	}
}

func TestParseDotenvErrors(t *testing.T) {
	cases := map[string]string{
		"no equals":         "PORT 3000\n",
		"bad key":           "1PORT=3000\n",
		"export prefix":     "export PORT=3000\n",
		"unclosed single":   "A='abc\n",
		"unclosed double":   `A="abc` + "\n",
		"trailing escape":   `A="abc\` + "\n",
		"unknown escape":    `A="a\qb"` + "\n",
		"text after quote":  `A="abc" def` + "\n",
		"text after single": "A='abc' def\n",
	}
	for name, in := range cases {
		_, err := ParseDotenv(strings.NewReader("OK=1\n" + in))
		if err == nil {
			t.Errorf("%s: expected an error", name)
		} else if !strings.Contains(err.Error(), "line 2") {
			t.Errorf("%s: error %q does not name the line", name, err)
		}
	}
}
