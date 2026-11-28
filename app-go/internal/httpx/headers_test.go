package httpx

import (
	"net/http"
	"slices"
	"testing"
)

func TestReservedResponseHeader(t *testing.T) {
	for _, name := range []string{"Content-Length", "content-length", "TRANSFER-ENCODING", "Trailer", "Connection", "Keep-Alive", "Proxy-Connection", "TE", "upgrade"} {
		if !ReservedResponseHeader(name) {
			t.Errorf("%s is not reserved", name)
		}
	}
	for _, name := range []string{"Content-Type", "X-Content-Length", "Cache-Control", "Etag", "Content-Encoding", ""} {
		if ReservedResponseHeader(name) {
			t.Errorf("%s is reserved", name)
		}
	}
}

func TestRemoveCookie(t *testing.T) {
	cases := []struct {
		in   []string
		want []string // nil: no Cookie header left
	}{
		{[]string{"tmock_session=abc"}, nil},
		{[]string{"a=1; tmock_session=abc; b=2"}, []string{"a=1; b=2"}},
		{[]string{"tmock_session=abc", "c=3"}, []string{"c=3"}}, // HTTP/2 sends several
		{[]string{" tmock_session = abc ;a=1"}, []string{"a=1"}},
		{[]string{"a=1;  b=2"}, []string{"a=1;  b=2"}}, // untouched when absent
		{[]string{"x_tmock_session=1; tmock_sessionx=2"}, []string{"x_tmock_session=1; tmock_sessionx=2"}},
	}
	for _, tc := range cases {
		h := http.Header{"Cookie": tc.in}
		RemoveCookie(h, SessionCookieName)
		if got := h["Cookie"]; !slices.Equal(got, tc.want) {
			t.Errorf("%q: got %q, want %q", tc.in, got, tc.want)
		}
	}
	h := http.Header{}
	RemoveCookie(h, SessionCookieName)
	if len(h) != 0 {
		t.Errorf("a header was added: %v", h)
	}
}

func TestRemoveSetCookie(t *testing.T) {
	h := http.Header{"Set-Cookie": {"tmock_session=; Max-Age=0; Path=/", "a=1; Path=/", " tmock_session=x"}}
	RemoveSetCookie(h, SessionCookieName)
	if got := h["Set-Cookie"]; !slices.Equal(got, []string{"a=1; Path=/"}) {
		t.Errorf("got %q", got)
	}
	h = http.Header{"Set-Cookie": {"tmock_session=x"}}
	RemoveSetCookie(h, SessionCookieName)
	if _, ok := h["Set-Cookie"]; ok {
		t.Errorf("got %q", h["Set-Cookie"])
	}
}
