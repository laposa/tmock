package httpx

import "net/http"

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
