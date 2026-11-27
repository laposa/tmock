package expr

import (
	"errors"
	"net/url"
	"strings"
	"testing"
)

func testRequest() *Request {
	return &Request{
		Method:  "POST",
		Path:    "/api/proxy/users/orders/42",
		Query:   map[string]string{"page": "2", "empty": ""},
		Headers: map[string]string{"content-type": "application/json", "x-env": "qa"},
		Body: BodyValue("application/json", []byte(`{
			"userId": "4a30d84d-b820-4396-81b3-bc010aa95c01",
			"count": 3, "zero": 0, "flag": true, "none": null,
			"items": [{"sku": "a"}, {"sku": "b"}],
			"nested": {"deep": {"x": "y"}},
			"big": 9007199254740993, "huge": 123456789012345678901234567890,
			"dec": 0.1, "exp": 1e3, "nums": [1, 2.50], "nums2": [1.0, 2.5], "inf": 1e999
		}`)),
		IP: "127.0.0.1",
	}
}

// Spec §7.4: grammar and semantics.
func TestEval(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src  string
		want bool
	}{
		// The legacy seed example.
		{`$request.body.userId == "4a30d84d-b820-4396-81b3-bc010aa95c01"`, true},
		{`$request.body.userId === "4a30d84d-b820-4396-81b3-bc010aa95c01"`, true},
		{`$request.body.userId != "4a30d84d-b820-4396-81b3-bc010aa95c01"`, false},
		{`$request.body.userId !== 'other'`, true},
		// The namespace.
		{`$request.method == 'POST'`, true},
		{`$request.method == 'post'`, false},
		{`$request.path == '/api/proxy/users/orders/42'`, true},
		{`$request.query.page == '2'`, true},
		{`$request.query["page"] == "2"`, true},
		{`$request.headers["content-type"] == 'application/json'`, true},
		{`$request.headers['x-env'] == 'qa'`, true},
		{`$request.ip == '127.0.0.1'`, true},
		{`$request.body.items[1].sku == 'b'`, true},
		{`$request.body["nested"]["deep"].x == 'y'`, true},
		// Missing paths are null.
		{`$request.body.missing == null`, true},
		{`$request.body.missing.deeper == null`, true},
		{`$request.body.items[5] == null`, true},
		{`$request.body.items.sku == null`, true},
		{`$request.body.nested[0] == null`, true},
		{`$request.query.missing == null`, true},
		{`$request.query.page.x == null`, true},
		{`$request.unknown == null`, true},
		{`$request.body.none == null`, true},
		{`$request.body.missing`, false},
		// Truthiness.
		{`$request.body.flag`, true},
		{`$request.body.zero`, false},
		{`$request.body.count`, true},
		{`$request.query.empty`, false},
		{`$request.body.items`, true},
		{`$request`, true},
		{`!$request.body.zero`, true},
		{`!!$request.body.count`, true},
		{`!$request.body.missing`, true},
		{`null`, false},
		{`0`, false},
		{`''`, false},
		{`'x'`, true},
		// ! binds to the comparison: !(a == b).
		{`!$request.method == 'GET'`, true},
		{`!$request.method == 'POST'`, false},
		// Types: "3" is not 3.
		{`$request.body.count == 3`, true},
		{`$request.body.count == 3.0`, true},
		{`$request.body.count == '3'`, false},
		{`$request.query.page == 2`, false},
		{`$request.body.flag == true`, true},
		{`$request.body.flag == 'true'`, false},
		{`null == false`, false},
		{`0 == false`, false},
		{`$request.body.items == $request.body.items`, true},
		{`$request.body.nested == $request.body.items`, false},
		// Numbers are exact beyond float64's 2^53.
		{`$request.body.big == 9007199254740993`, true},
		{`$request.body.big == 9007199254740992`, false},
		{`$request.body.big > 9007199254740992`, true},
		{`$request.body.huge == 123456789012345678901234567890`, true},
		{`$request.body.huge == 123456789012345678901234567891`, false},
		{`$request.body.dec == 0.1`, true},
		{`$request.body.exp == 1000`, true},
		{`$request.body.nums == $request.body.nums2`, true}, // 1 is 1.0 inside arrays too
		{`$request.body.inf > 1e300`, true},
		// Ordering: numbers with numbers, strings with strings, nothing else.
		{`$request.body.count > 2`, true},
		{`$request.body.count >= 3`, true},
		{`$request.body.count < 3`, false},
		{`$request.body.count <= -1`, false},
		{`$request.body.count > -1.5e2`, true},
		{`$request.body.count > '2'`, false},
		{`$request.body.count < '4'`, false},
		{`$request.query.page > '10'`, true}, // strings compare as strings
		{`$request.query.page < 10`, false},
		{`null < 1`, false},
		{`null >= null`, false},
		{`true > false`, false},
		// && and ||, precedence and parentheses.
		{`$request.method == 'POST' && $request.query.page == '2'`, true},
		{`$request.method == 'GET' && $request.query.page == '2'`, false},
		{`$request.method == 'GET' || $request.query.page == '2'`, true},
		{`$request.method == 'GET' || $request.method == 'PUT'`, false},
		{`true || true && false`, true},
		{`(true || true) && false`, false},
		{`!(true && false)`, true},
		{`!(($request.method == 'GET'))`, true},
		{"$request.method == 'POST'\n  && $request.ip == '127.0.0.1'", true},
		// Escapes.
		{`'it\'s' == "it's"`, true},
		{`"a\"b" == 'a"b'`, true},
		{`'é' == 'é'`, true},
		{`'a\\b' == "a\\b"`, true},
	}
	req := testRequest()
	for _, tc := range cases {
		e, err := Parse(tc.src)
		if err != nil {
			t.Errorf("%s: %v", tc.src, err)
			continue
		}
		if got := e.Eval(req); got != tc.want {
			t.Errorf("%s: %v, want %v", tc.src, got, tc.want)
		}
	}
}

func TestEvalEmptyRequest(t *testing.T) {
	t.Parallel()
	for _, src := range []string{`$request.body.userId == 'x'`, `$request.query.a`, `$request.headers.x`, `$request.body[0]`} {
		e, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		if e.Eval(&Request{}) || e.Eval(nil) {
			t.Errorf("%s: true for an empty request", src)
		}
	}
}

// Spec §7.4: errors carry their position.
func TestParseErrors(t *testing.T) {
	t.Parallel()
	cases := []struct {
		src     string
		line    int
		column  int
		message string
	}{
		{``, 1, 1, "expected a value but found end of condition"},
		{`   `, 1, 4, "expected a value but found end of condition"},
		{`$request.method ==`, 1, 19, "expected a value but found end of condition"},
		{`$request.method = 'GET'`, 1, 17, `"=" is not an operator; use "==" to compare`},
		{`$req.method == 'GET'`, 1, 1, `unknown name "$req": only $request is available`},
		{`request.method == 'GET'`, 1, 1, `unknown name "request": values come from $request, strings need quotes`},
		{`$request.method == GET`, 1, 20, `unknown name "GET": values come from $request, strings need quotes`},
		{`($request.method == 'GET'`, 1, 26, `expected ")" but found end of condition`},
		{`$request.method == 'GET')`, 1, 25, `unexpected ")"`},
		{`$request.method == 'GET`, 1, 20, "unterminated string"},
		{`$request.`, 1, 10, `expected a name after "." but found end of condition`},
		{`$request.body[1.5]`, 1, 15, `expected a string or a whole number in "[]" but found number 1.5`},
		{`$request.body[-1]`, 1, 15, `expected a string or a whole number in "[]" but found number -1`},
		{`$request.body['a'`, 1, 18, `expected "]" but found end of condition`},
		{`$request.body.a == 1 == 2`, 1, 22, "comparisons cannot be chained; use && or parentheses"},
		{`$request.body.a & 1`, 1, 17, `unexpected character '&'`},
		{`$request.body.a == 1abc`, 1, 21, `unexpected 'a' after a number`},
		{`$request.body.a == 1.2.3`, 1, 20, `"1.2.3" is not a number`},
		{`$request.body.a == '\x'`, 1, 21, `unknown escape "\\x"`},
		{`'é' == ü`, 1, 8, `unexpected character 'ü'`},
		{"$request.method == 'GET'\n&& $request.x ==", 2, 17, "expected a value but found end of condition"},
		{`$request.method $request.path`, 1, 17, `unexpected $request`},
		{`!`, 1, 2, "expected a value but found end of condition"},
		{`$request.a == 1.`, 1, 15, `"1." is not a number`},
		{`$request.a == 1.e5`, 1, 15, `"1.e5" is not a number`},
		{`$request.a == 1e`, 1, 15, `"1e" is not a number`},
		{`$request.a == 1e999999999`, 1, 15, `"1e999999999" is not a number`},
		{`$request.a == .5`, 1, 15, `expected a value but found "."`},
		{`$request.a[99999999999999999999]`, 1, 12, `index 99999999999999999999 is too large`},
		{`()`, 1, 2, `expected a value but found ")"`},
	}
	for _, tc := range cases {
		_, err := Parse(tc.src)
		var e *Error
		if !errors.As(err, &e) {
			t.Errorf("%q: error %v, want an *Error", tc.src, err)
			continue
		}
		if e.Line != tc.line || e.Column != tc.column || e.Msg != tc.message {
			t.Errorf("%q: %d:%d %q, want %d:%d %q", tc.src, e.Line, e.Column, e.Msg, tc.line, tc.column, tc.message)
		}
	}
}

func TestErrorAt(t *testing.T) {
	t.Parallel()
	e := ErrorAt("  a\n  bé c", 9, "x")
	if e.Line != 2 || e.Column != 5 || e.Offset != 9 {
		t.Errorf("%+v", e)
	}
	if e := ErrorAt("ab", 99, "x"); e.Offset != 2 || e.Column != 3 {
		t.Errorf("past the end: %+v", e)
	}
}

func TestValidNumber(t *testing.T) {
	t.Parallel()
	for s, want := range map[string]bool{"0": true, "-1": true, "01": true, "1.5": true, "1e5": true, "1E-5": true, "2.5e+3": true,
		"": false, "-": false, "1.": false, ".5": false, "1e": false, "1e+": false, "1.e5": false, "--1": false, "1x": false} {
		if validNumber(s) != want {
			t.Errorf("%q: %v", s, !want)
		}
	}
}

// A number of a megabyte is parsed in linear time (as float64), not exactly.
func TestLongNumber(t *testing.T) {
	t.Parallel()
	body := []byte(`{"n":1` + strings.Repeat("0", 1<<20) + `}`)
	v, _ := BodyValue("application/json", body).(map[string]any)
	if n, ok := toNum(v["n"]); !ok || !n.IsInf() {
		t.Errorf("%v", v["n"])
	}
}

func TestErrorString(t *testing.T) {
	t.Parallel()
	_, err := Parse(`$request.method = 'GET'`)
	if got, want := err.Error(), `column 17: "=" is not an operator; use "==" to compare`; got != want {
		t.Errorf("%q, want %q", got, want)
	}
	_, err = Parse("true &&\n")
	if got, want := err.Error(), "line 2, column 1: expected a value but found end of condition"; got != want {
		t.Errorf("%q, want %q", got, want)
	}
}

func TestMaxDepth(t *testing.T) {
	t.Parallel()
	nest := func(n int, open, close string) string {
		s := "true"
		for range n {
			s = open + s + close
		}
		return s
	}
	for _, tc := range []struct{ open, close string }{{"(", ")"}, {"!", ""}} {
		if _, err := Parse(nest(MaxDepth, tc.open, tc.close)); err != nil {
			t.Errorf("%s × %d: %v", tc.open, MaxDepth, err)
		}
		_, err := Parse(nest(MaxDepth+1, tc.open, tc.close))
		var e *Error
		if !errors.As(err, &e) || e.Msg != "the condition is nested too deeply" {
			t.Errorf("%s × %d: %v", tc.open, MaxDepth+1, err)
		}
	}
}

func TestUsesBody(t *testing.T) {
	t.Parallel()
	for src, want := range map[string]bool{
		`$request.method == 'GET'`:                   false,
		`$request.headers.x == 'a' && $request.ip`:   false,
		`$request.body.userId == 'x'`:                true,
		`$request.method == 'GET' || $request.body`:  true,
		`$request["body"].a`:                         true,
		`$request`:                                   true,
		`!($request.query.a == $request.body.items)`: true,
	} {
		e, err := Parse(src)
		if err != nil {
			t.Fatal(err)
		}
		if e.UsesBody() != want {
			t.Errorf("%s: UsesBody %v, want %v", src, e.UsesBody(), want)
		}
	}
}

func TestBodyValue(t *testing.T) {
	t.Parallel()
	cases := []struct {
		ct, body string
		path     string
		want     any
	}{
		{"application/json", `{"a":"b"}`, "a", "b"},
		{"application/json; charset=utf-8", `{"a":1}`, "a", 1.0},
		{"application/vnd.api+json", `{"a":true}`, "a", true},
		{"", `{"a":"b"}`, "a", "b"}, // no content type: JSON if it parses
		{"application/json", `not json`, "", nil},
		{"application/json", ``, "", nil},
		{"application/json", `{"a":1} trailing`, "", nil},
		{"application/json", `{"a":1} {}`, "", nil},
		{"application/json", " {\"a\":1}\n", "a", 1.0},
		{"application/x-www-form-urlencoded", `a=1&a=2&b=x%20y`, "a", "1"},
		{"application/x-www-form-urlencoded", `a=1&a=2&b=x%20y`, "b", "x y"},
		{"text/plain", `{"a":"b"}`, "", nil},
		{"multipart/form-data; boundary=x", `--x--`, "", nil},
		{"bad;;type", `{"a":"b"}`, "", nil},
	}
	for _, tc := range cases {
		v := BodyValue(tc.ct, []byte(tc.body))
		if tc.path != "" {
			m, _ := v.(map[string]any)
			v = m[tc.path]
		}
		if !equal(v, tc.want) {
			t.Errorf("%q %q: %#v, want %#v", tc.ct, tc.body, v, tc.want)
		}
	}
}

func TestFirstValues(t *testing.T) {
	t.Parallel()
	got := FirstValues(url.Values{"a": {"1", "2"}, "b": {""}, "c": {}})
	if len(got) != 2 || got["a"] != "1" || got["b"] != "" {
		t.Errorf("%v", got)
	}
}

// Parse and Eval never panic, whatever the input (spec §11.1).
func FuzzParse(f *testing.F) {
	for _, s := range []string{
		`$request.body.userId == "4a30d84d-b820-4396-81b3-bc010aa95c01"`,
		`!($request.method == 'GET' || $request.query["a"] >= 2) && $request.body.items[0].x`,
		`'é' === "\n" !== null`, `1e5 < -2.5`, `((((`, `$request[`, `"\u12`,
	} {
		f.Add(s)
	}
	req := testRequest()
	f.Fuzz(func(t *testing.T, src string) {
		e, err := Parse(src)
		if err != nil {
			var pe *Error
			if !errors.As(err, &pe) || pe.Offset < 0 || pe.Offset > len(src) || pe.Line < 1 || pe.Column < 1 {
				t.Fatalf("%q: bad error %#v", src, err)
			}
			return
		}
		e.Eval(req)
		e.Eval(&Request{})
	})
}
