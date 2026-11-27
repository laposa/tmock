package liquid_test

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/laposa/tmock/app-go/internal/liquid"
)

// caseNow is 'now' in testdata/cases.txt.
var caseNow = time.Date(2024, 2, 29, 13, 5, 9, 123e6, time.UTC)

type testCase struct {
	name, template, want string
	wantErr              bool
	line                 int
}

// readCases reads testdata/cases.txt (its format is described at the top of the file).
func readCases(t testing.TB) []testCase {
	t.Helper()
	f, err := os.Open("testdata/cases.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var cases []testCase
	var cur *testCase
	var tmpl, want []string
	inWant := false
	flush := func() {
		if cur == nil {
			return
		}
		cur.template = strings.TrimRight(strings.Join(tmpl, "\n"), "\n")
		cur.want = strings.TrimRight(strings.Join(want, "\n"), "\n")
		cases = append(cases, *cur)
	}
	sc := bufio.NewScanner(f)
	for n := 1; sc.Scan(); n++ {
		line := sc.Text()
		switch {
		case strings.HasPrefix(line, "=== "):
			flush()
			cur = &testCase{name: strings.TrimPrefix(line, "=== "), line: n}
			tmpl, want, inWant = nil, nil, false
		case cur == nil:
			// The header.
		case !inWant && (line == "---" || line == "--- error"):
			inWant, cur.wantErr = true, line == "--- error"
		case inWant:
			want = append(want, line)
		default:
			tmpl = append(tmpl, line)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	flush()
	for _, c := range cases {
		if c.template == "" && !c.wantErr {
			t.Fatalf("cases.txt:%d: %s has no template", c.line, c.name)
		}
	}
	return cases
}

func render(src string, opts liquid.Options) (string, error) {
	tpl, err := liquid.Parse(src)
	if err != nil {
		return "", err
	}
	return tpl.Render(opts)
}

func TestCases(t *testing.T) {
	t.Parallel()
	cases := readCases(t)
	if len(cases) < 50 {
		t.Fatalf("only %d cases read", len(cases))
	}
	opts := liquid.Options{Now: func() time.Time { return caseNow }, Location: time.UTC}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			got, err := render(c.template, opts)
			switch {
			case c.wantErr && err == nil:
				t.Fatalf("cases.txt:%d: got %q, want the error %q", c.line, got, c.want)
			case c.wantErr && err.Error() != c.want:
				t.Fatalf("cases.txt:%d: error\n got: %s\nwant: %s", c.line, err, c.want)
			case !c.wantErr && err != nil:
				t.Fatalf("cases.txt:%d: %v", c.line, err)
			case !c.wantErr && got != c.want:
				t.Fatalf("cases.txt:%d:\n got: %q\nwant: %q", c.line, got, c.want)
			}
			var e *liquid.Error
			if err != nil && !errors.As(err, &e) {
				t.Fatalf("error %T is not a *liquid.Error", err)
			}
		})
	}
}

// TestLegacyExamplesNow renders the spec §8.3 examples against the real clock, checked
// structurally: the format, and within 2 seconds of now.
func TestLegacyExamplesNow(t *testing.T) {
	t.Parallel()
	before := time.Now()
	body := liquid.Marker + `{% assign seconds = 5 | times: 24 | times: 60 | times: 60 %}{"validFrom": "{{ 'now' | date: "%s" | plus: seconds | date: "%Y-%m-%d" }}"}`
	tpl, err := liquid.ParseBody(body)
	if err != nil {
		t.Fatal(err)
	}
	got, err := tpl.Render(liquid.Options{})
	if err != nil {
		t.Fatal(err)
	}
	after := time.Now()
	ok := false
	for _, ref := range []time.Time{before, after} {
		ok = ok || got == `{"validFrom": "`+ref.AddDate(0, 0, 5).Format(time.DateOnly)+`"}`
	}
	if !ok {
		t.Errorf("validFrom = %s, want today + 5 days (%s)", got, before.AddDate(0, 0, 5).Format(time.DateOnly))
	}

	got, err = render(`{{ 'now' | date: "%Y-%m-%dT%H:%M:%SZ" }}`, liquid.Options{})
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^\d{4}-\d\d-\d\dT\d\d:\d\d:\d\dZ$`).MatchString(got) {
		t.Fatalf("now = %q, not in the format", got)
	}
	// The format says Z, but the time is local (as in LiquidJS).
	at, err := time.ParseInLocation("2006-01-02T15:04:05Z", got, time.Local)
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Since(at); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("now = %s, %s away from the clock", got, d)
	}
}

// TestNowOncePerRender: every use of 'now' in one render sees the same time.
func TestNowOncePerRender(t *testing.T) {
	t.Parallel()
	calls := 0
	opts := liquid.Options{Now: func() time.Time { calls++; return caseNow.Add(time.Duration(calls) * time.Second) }, Location: time.UTC}
	got, err := render(`{{ 'now' | date: "%S" }}{{ 'today' | date: "%S" }}`, opts)
	if err != nil || got != "1010" || calls != 1 {
		t.Errorf("got %q, %v after %d calls", got, err, calls)
	}
}

func TestParseBody(t *testing.T) {
	t.Parallel()
	if tpl, err := liquid.ParseBody(`{"a": 1}`); tpl != nil || err != nil {
		t.Errorf("a plain body parsed: %v, %v", tpl, err)
	}
	if !liquid.IsTemplate(liquid.Marker+"x") || liquid.IsTemplate(" "+liquid.Marker) {
		t.Error("IsTemplate")
	}
	tpl, err := liquid.ParseBody(liquid.Marker + `{{ "a" | upcase }}`)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := tpl.Render(liquid.Options{}); got != "A" {
		t.Errorf("rendered %q, want the marker removed", got)
	}
	// Positions count the marker, as the body is shown with it.
	_, err = liquid.ParseBody(liquid.Marker + `{{ a | }}`)
	if want := `line 1, column 32: expected a filter name after "|", but found nothing`; err == nil || err.Error() != want {
		t.Errorf("error %v, want %s", err, want)
	}
	_, err = liquid.ParseBody(liquid.Marker + "\n{% if %}{% endif %}")
	if want := "line 2, column 7: expected a value but found nothing"; err == nil || err.Error() != want {
		t.Errorf("error %v, want %s", err, want)
	}
}

func TestDepthLimit(t *testing.T) {
	t.Parallel()
	nest := func(n int) string {
		return strings.Repeat("{% if true %}", n) + "x" + strings.Repeat("{% endif %}", n)
	}
	if got, err := render(nest(liquid.MaxDepth), liquid.Options{}); err != nil || got != "x" {
		t.Errorf("%d levels: %q, %v", liquid.MaxDepth, got, err)
	}
	if _, err := render(nest(liquid.MaxDepth+1), liquid.Options{}); err == nil || !strings.Contains(err.Error(), "nest more than 64 deep") {
		t.Errorf("%d levels: %v", liquid.MaxDepth+1, err)
	}
	idx := "{{ a" + strings.Repeat("[a", liquid.MaxDepth+1) + strings.Repeat("]", liquid.MaxDepth+1) + " }}"
	if _, err := render(idx, liquid.Options{}); err == nil || !strings.Contains(err.Error(), "nest more than 64 deep") {
		t.Errorf("deep [ ]: %v", err)
	}
}

// TestConcurrentRenders: a template is shared by all requests of a catalog snapshot.
func TestConcurrentRenders(t *testing.T) {
	t.Parallel()
	tpl, err := liquid.Parse(`{% assign n = 0 %}{% for i in (1..50) %}{% assign n = n | plus: i %}{% endfor %}{{ n }}`)
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan string)
	for range 8 {
		go func() {
			out, _ := tpl.Render(liquid.Options{})
			done <- out
		}()
	}
	for range 8 {
		if got := <-done; got != "1275" {
			t.Errorf("got %q", got)
		}
	}
}

// TestLocalZoneFromTZ runs the test binary again with TZ=Europe/Dublin: dates are shown
// in the process's local zone, with its summer time (time/tzdata is embedded for systems
// without a zone database).
func TestLocalZoneFromTZ(t *testing.T) {
	const child = "LIQUID_TZ_CHILD"
	if os.Getenv(child) == "1" {
		got, err := render(`{{ "2026-07-01T12:00:00Z" | date: "%H:%M %Z %z" }}|{{ "2026-01-15T12:00:00Z" | date: "%H:%M %Z %z" }}|{{ "2026-07-01" | date: "%s" }}`, liquid.Options{})
		if err != nil {
			got = err.Error()
		}
		fmt.Println("RESULT:" + got)
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestLocalZoneFromTZ$", "-test.count=1")
	cmd.Env = append(os.Environ(), child+"=1", "TZ=Europe/Dublin")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	_, result, ok := strings.Cut(string(out), "RESULT:")
	if !ok {
		t.Fatalf("no result in %s", out)
	}
	result, _, _ = strings.Cut(result, "\n")
	// Midnight of 1 July in Dublin is 23:00 UTC on 30 June.
	want := "13:00 IST +0100|12:00 GMT +0000|" + strconv.FormatInt(time.Date(2026, 6, 30, 23, 0, 0, 0, time.UTC).Unix(), 10)
	if result != want {
		t.Errorf("in Europe/Dublin: %s, want %s", result, want)
	}
}

func FuzzParse(f *testing.F) {
	for _, c := range readCases(f) {
		f.Add(c.template)
	}
	f.Add(liquid.Marker + "{{ x")
	f.Add("{%- raw -%}{%- endraw -%}{{- -}}")
	opts := liquid.Options{Now: func() time.Time { return caseNow }, Location: time.UTC}
	f.Fuzz(func(t *testing.T, src string) {
		tpl, err := liquid.Parse(src)
		if err != nil {
			var e *liquid.Error
			if !errors.As(err, &e) || e.Offset < 0 || e.Offset > len(src) || e.Line < 1 || e.Column < 1 {
				t.Fatalf("bad error %#v", err)
			}
			return
		}
		out, err := tpl.Render(opts)
		if err == nil && len(out) > liquid.MaxSize {
			t.Fatalf("output of %d bytes", len(out))
		}
	})
}
