// Package clientip determines the IP address of the client that sent a request (spec §7.3).
// The same rule is used for logging, client conditions and login events.
package clientip

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

// From returns the first non-empty value of cf-connecting-ip, x-real-ip and the first
// entry of x-forwarded-for, falling back to the socket's remote address without the port.
// The value is trimmed, and IPv4-mapped IPv6 addresses (::ffff:a.b.c.d) become IPv4.
//
// The headers are trusted as sent; tmock is expected to run behind a proxy that sets
// them, as the legacy app did.
func From(r *http.Request) string {
	for _, h := range []string{"Cf-Connecting-Ip", "X-Real-Ip"} {
		if v := strings.TrimSpace(r.Header.Get(h)); v != "" {
			return Normalize(v)
		}
	}
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		first, _, _ := strings.Cut(xff, ",")
		if v := strings.TrimSpace(first); v != "" {
			return Normalize(v)
		}
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return Normalize(strings.TrimSpace(host))
}

// Normalize turns an IPv4-mapped IPv6 address into its IPv4 form. Anything that is not an
// IP address is returned unchanged.
func Normalize(s string) string {
	addr, err := netip.ParseAddr(s)
	if err != nil {
		return s
	}
	return addr.Unmap().String()
}
