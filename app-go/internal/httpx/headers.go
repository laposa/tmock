package httpx

import (
	"net/http"
	"slices"
	"strings"
)

// reservedResponseHeaders are the response headers a scenario may not set or delete
// (spec §6.4, found in I11, fixed in I13): the message framing, which tmock and Go's
// server work out from the body actually sent, and the hop-by-hop headers (RFC 9110
// §7.6.1), which describe one connection and are never forwarded. A scenario's
// Content-Length over a passed-through upstream body truncated or aborted the response.
var reservedResponseHeaders = map[string]bool{
	"Content-Length":    true,
	"Transfer-Encoding": true,
	"Trailer":           true,
	"Connection":        true,
	"Keep-Alive":        true,
	"Proxy-Connection":  true,
	"Te":                true,
	"Upgrade":           true,
}

// ReservedResponseHeader reports whether name, in any case, is a response header that a
// scenario may not set or delete.
func ReservedResponseHeader(name string) bool {
	return reservedResponseHeaders[http.CanonicalHeaderKey(name)]
}

// SessionCookieName is the dashboard's session cookie (spec §9.2). It is defined here so
// that the proxy can keep it away from upstreams without importing internal/auth.
const SessionCookieName = "tmock_session"

// RemoveCookie removes the cookie name from every Cookie header in h and leaves the other
// cookies as they were sent. A Cookie header left empty is removed.
func RemoveCookie(h http.Header, name string) {
	values, ok := h["Cookie"]
	if !ok {
		return
	}
	var kept []string
	removed := false
	for _, v := range values {
		var pairs []string
		for pair := range strings.SplitSeq(v, ";") {
			n, _, _ := strings.Cut(pair, "=")
			if strings.TrimSpace(n) == name {
				removed = true
			} else if p := strings.TrimSpace(pair); p != "" {
				pairs = append(pairs, p)
			}
		}
		if len(pairs) > 0 {
			kept = append(kept, strings.Join(pairs, "; "))
		}
	}
	switch {
	case !removed:
		// Untouched: the header goes on exactly as the client sent it.
	case len(kept) == 0:
		h.Del("Cookie")
	default:
		h["Cookie"] = kept
	}
}

// RemoveSetCookie removes every Set-Cookie header in h that sets the cookie name, so a
// response cannot replace or clear it.
func RemoveSetCookie(h http.Header, name string) {
	values, ok := h["Set-Cookie"]
	if !ok {
		return
	}
	kept := slices.DeleteFunc(slices.Clone(values), func(v string) bool {
		n, _, _ := strings.Cut(v, "=")
		return strings.TrimSpace(n) == name
	})
	switch {
	case len(kept) == len(values):
	case len(kept) == 0:
		h.Del("Set-Cookie")
	default:
		h["Set-Cookie"] = kept
	}
}
