package condition

import (
	"net/http"
	"net/netip"
	"testing"
)

func mustCompile(t *testing.T, text string) *Matcher {
	t.Helper()
	c, err := Parse(text)
	if err != nil {
		t.Fatalf("parse %s: %v", text, err)
	}
	m, err := Compile(c)
	if err != nil {
		t.Fatalf("compile %s: %v", text, err)
	}
	return m
}

func req(ip string, kv ...string) Request {
	r := Request{Header: http.Header{}}
	if ip != "" {
		r.IP = netip.MustParseAddr(ip).Unmap()
	}
	for i := 0; i+1 < len(kv); i += 2 {
		r.Header.Add(kv[i], kv[i+1])
	}
	return r
}

func TestMatch(t *testing.T) {
	cases := []struct {
		name, cond string
		req        Request
		want       bool
	}{
		{"ip equal", `{"ip":"10.0.0.1"}`, req("10.0.0.1"), true},
		{"ip other", `{"ip":"10.0.0.1"}`, req("10.0.0.2"), false},
		{"ip not", `{"ip":"10.0.0.1","not":true}`, req("10.0.0.2"), true},
		{"ip missing", `{"ip":"10.0.0.1"}`, req(""), false},
		{"ip mapped client", `{"ip":"10.0.0.1"}`, req("::ffff:10.0.0.1"), true},
		{"ip mapped value", `{"ip":"::ffff:10.0.0.1"}`, req("10.0.0.1"), true},
		{"ipv6", `{"ip":"2001:db8::1"}`, req("2001:db8:0:0::1"), true},

		{"cidr in", `{"cidr":"10.0.0.0/8"}`, req("10.200.3.4"), true},
		{"cidr out", `{"cidr":"10.0.0.0/8"}`, req("11.0.0.1"), false},
		{"cidr unmasked", `{"cidr":"10.1.2.3/8"}`, req("10.9.9.9"), true},
		{"cidr v6 in", `{"cidr":"2001:db8::/32"}`, req("2001:db8:ffff::1"), true},
		{"cidr v6 out", `{"cidr":"2001:db8::/32"}`, req("2001:db9::1"), false},
		{"cidr v4 vs v6 client", `{"cidr":"10.0.0.0/8"}`, req("2001:db8::1"), false},
		{"cidr mapped client", `{"cidr":"10.0.0.0/8"}`, req("::ffff:10.1.1.1"), true},
		{"cidr mapped prefix", `{"cidr":"::ffff:10.0.0.0/104"}`, req("10.1.1.1"), true},
		{"cidr not", `{"cidr":"10.0.0.0/8","not":true}`, req("10.1.1.1"), false},

		{"header equal", `{"headerMatch":{"header":"X-Env","value":"qa"}}`, req("", "X-Env", "qa"), true},
		{"header name case", `{"headerMatch":{"header":"x-env","value":"qa"}}`, req("", "X-ENV", "qa"), true},
		{"header value case", `{"headerMatch":{"header":"X-Env","value":"qa"}}`, req("", "X-Env", "QA"), false},
		{"header missing", `{"headerMatch":{"header":"X-Env","value":"qa"}}`, req(""), false},
		{"header missing not", `{"headerMatch":{"header":"X-Env","value":"qa"},"not":true}`, req(""), true},
		{"header joined", `{"headerMatch":{"header":"X-Env","value":"a, b"}}`, req("", "X-Env", "a", "X-Env", "b"), true},
		{"header one of several", `{"headerMatch":{"header":"X-Env","value":"a"}}`, req("", "X-Env", "a", "X-Env", "b"), false},

		{"regex unanchored", `{"headerRegex":{"header":"User-Agent","value":"Mobile"}}`, req("", "User-Agent", "Foo Mobile Bar"), true},
		{"regex anchored", `{"headerRegex":{"header":"User-Agent","value":"^Mobile$"}}`, req("", "User-Agent", "Foo Mobile"), false},
		{"regex missing is empty", `{"headerRegex":{"header":"X-A","value":"^$"}}`, req(""), true},
		{"regex missing", `{"headerRegex":{"header":"X-A","value":"x"}}`, req(""), false},
		{"regex joined", `{"headerRegex":{"header":"X-A","value":"^a, b$"}}`, req("", "X-A", "a", "X-A", "b"), true},

		{"and empty", `{"and":[]}`, req(""), true},
		{"or empty", `{"or":[]}`, req(""), false},
		{"and empty not", `{"and":[],"not":true}`, req(""), false},
		{"or empty not", `{"or":[],"not":true}`, req(""), true},
		{"and all", `{"and":[{"ip":"10.0.0.1"},{"headerMatch":{"header":"X","value":"1"}}]}`, req("10.0.0.1", "X", "1"), true},
		{"and one false", `{"and":[{"ip":"10.0.0.1"},{"headerMatch":{"header":"X","value":"1"}}]}`, req("10.0.0.1"), false},
		{"or one", `{"or":[{"ip":"10.0.0.9"},{"cidr":"10.0.0.0/24"}]}`, req("10.0.0.1"), true},
		{"or none", `{"or":[{"ip":"10.0.0.9"},{"cidr":"10.1.0.0/24"}]}`, req("10.0.0.1"), false},
		// The I08 manual example: NOT (ip = 10.0.0.1 OR (header X-Env = qa AND cidr 192.168.0.0/16))
		{"nested 1", `{"not":true,"or":[{"ip":"10.0.0.1"},{"and":[{"headerMatch":{"header":"X-Env","value":"qa"}},{"cidr":"192.168.0.0/16"}]}]}`, req("192.168.1.1", "X-Env", "qa"), false},
		{"nested 2", `{"not":true,"or":[{"ip":"10.0.0.1"},{"and":[{"headerMatch":{"header":"X-Env","value":"qa"}},{"cidr":"192.168.0.0/16"}]}]}`, req("192.168.1.1", "X-Env", "prod"), true},
		{"nested 3", `{"not":true,"or":[{"ip":"10.0.0.1"},{"and":[{"headerMatch":{"header":"X-Env","value":"qa"}},{"cidr":"192.168.0.0/16"}]}]}`, req("10.0.0.1"), false},
	}
	for _, tc := range cases {
		if got := mustCompile(t, tc.cond).Match(tc.req); got != tc.want {
			t.Errorf("%s: %s → %v, want %v", tc.name, tc.cond, got, tc.want)
		}
	}
}

// A header name the server could not canonicalise is still found, ignoring case.
func TestMatchUncanonicalHeader(t *testing.T) {
	m := mustCompile(t, `{"headerMatch":{"header":"x_env","value":"qa"}}`)
	if !m.Match(Request{Header: http.Header{"X_ENV": {"qa"}}}) {
		t.Error("not matched")
	}
}

func TestCompileRejectsInvalid(t *testing.T) {
	for _, text := range []string{`{"ip":"nope"}`, `{"headerRegex":{"header":"X","value":"("}}`, `{}`} {
		c, err := Parse(text)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := Compile(c); err == nil {
			t.Errorf("%s compiled", text)
		}
	}
	if _, err := Compile(nil); err == nil {
		t.Error("nil compiled")
	}
}

// Go's server keeps Host out of Header; conditions on it still work.
func TestMatchHost(t *testing.T) {
	m := mustCompile(t, `{"headerMatch":{"header":"host","value":"qa.example.com"}}`)
	if !m.Match(Request{Header: http.Header{}, Host: "qa.example.com"}) {
		t.Error("Host not matched")
	}
	if m.Match(Request{Header: http.Header{}, Host: "prod.example.com"}) {
		t.Error("other Host matched")
	}
	re := mustCompile(t, `{"headerRegex":{"header":"Host","value":"^qa\\."}}`)
	if !re.Match(Request{Header: http.Header{}, Host: "qa.example.com"}) {
		t.Error("Host regex not matched")
	}
}

// An IPv6 prefix that covers the IPv4-mapped range covers IPv4 clients too (spec §7.3:
// normalised on both sides); other IPv6 prefixes do not.
func TestCIDRv6PrefixCoveringIPv4(t *testing.T) {
	for cond, want := range map[string]bool{
		`{"cidr":"::/0"}`:                true,
		`{"cidr":"::/80"}`:               true,
		`{"cidr":"::ffff:0:0/96"}`:       true,
		`{"cidr":"2001:db8::/32"}`:       false,
		`{"cidr":"::ffff:10.0.0.0/104"}`: true,
		`{"cidr":"::ffff:11.0.0.0/104"}`: false,
	} {
		if got := mustCompile(t, cond).Match(req("10.1.2.3")); got != want {
			t.Errorf("%s with 10.1.2.3: %v, want %v", cond, got, want)
		}
	}
}
