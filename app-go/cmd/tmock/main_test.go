package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRunCommands(t *testing.T) {
	noEnv := func(string) string { return "" }
	cases := []struct {
		args       []string
		wantCode   int
		wantStdout string
		wantStderr string
	}{
		{[]string{"help"}, 0, "Usage: tmock", ""},
		{[]string{"--help"}, 0, "Usage: tmock", ""},
		{[]string{"bogus"}, 2, "", `unknown command "bogus"`},
		{[]string{"serve", "extra"}, 2, "", "serve takes no arguments"},
	}
	for _, tc := range cases {
		var out, errOut bytes.Buffer
		code := run(tc.args, noEnv, &out, &errOut)
		if code != tc.wantCode {
			t.Errorf("%v: exit code %d, want %d", tc.args, code, tc.wantCode)
		}
		if !strings.Contains(out.String(), tc.wantStdout) || !strings.Contains(errOut.String(), tc.wantStderr) {
			t.Errorf("%v: stdout %q, stderr %q", tc.args, out.String(), errOut.String())
		}
	}
}

func TestServeRejectsBadPort(t *testing.T) {
	var errOut bytes.Buffer
	env := func(k string) string {
		if k == "PORT" {
			return "nope"
		}
		return ""
	}
	if code := run(nil, env, &bytes.Buffer{}, &errOut); code != 1 {
		t.Errorf("exit code %d, want 1", code)
	}
	if !strings.Contains(errOut.String(), "PORT must be a number") {
		t.Errorf("stderr %q", errOut.String())
	}
}

func TestParsePort(t *testing.T) {
	for in, want := range map[string]int{"": 3000, "8080": 8080, "1": 1, "65535": 65535} {
		if got, err := parsePort(in); err != nil || got != want {
			t.Errorf("parsePort(%q) = %d, %v; want %d", in, got, err, want)
		}
	}
	for _, in := range []string{"0", "65536", "-1", "80a", " 80"} {
		if _, err := parsePort(in); err == nil {
			t.Errorf("parsePort(%q): expected an error", in)
		}
	}
}
