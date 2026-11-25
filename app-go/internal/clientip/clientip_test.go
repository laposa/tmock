package clientip

import (
	"net/http/httptest"
	"testing"
)

func TestFrom(t *testing.T) {
	cases := []struct {
		name    string
		headers map[string]string
		remote  string
		want    string
	}{
		{"remote address", nil, "192.0.2.1:54321", "192.0.2.1"},
		{"remote IPv6", nil, "[2001:db8::1]:54321", "2001:db8::1"},
		{"remote IPv4-mapped", nil, "[::ffff:192.0.2.7]:80", "192.0.2.7"},
		{"cloudflare first", map[string]string{"CF-Connecting-IP": "198.51.100.1", "X-Real-IP": "198.51.100.2", "X-Forwarded-For": "198.51.100.3"}, "10.0.0.1:1", "198.51.100.1"},
		{"x-real-ip next", map[string]string{"X-Real-IP": " 198.51.100.2 ", "X-Forwarded-For": "198.51.100.3"}, "10.0.0.1:1", "198.51.100.2"},
		{"first forwarded-for entry", map[string]string{"X-Forwarded-For": " 198.51.100.3 , 10.0.0.2"}, "10.0.0.1:1", "198.51.100.3"},
		{"empty headers are skipped", map[string]string{"CF-Connecting-IP": " ", "X-Forwarded-For": ", 10.0.0.2"}, "10.0.0.1:1", "10.0.0.1"},
		{"mapped in header", map[string]string{"X-Real-IP": "::ffff:198.51.100.9"}, "10.0.0.1:1", "198.51.100.9"},
		{"not an IP is skipped", map[string]string{"X-Real-IP": "unknown", "X-Forwarded-For": "198.51.100.4"}, "10.0.0.1:1", "198.51.100.4"},
		{"injected text is skipped", map[string]string{"CF-Connecting-IP": "1.2.3.4\nfake log line"}, "10.0.0.1:1", "10.0.0.1"},
		{"remote without port", nil, "192.0.2.5", "192.0.2.5"},
	}
	for _, tc := range cases {
		r := httptest.NewRequest("GET", "/", nil)
		r.RemoteAddr = tc.remote
		for k, v := range tc.headers {
			r.Header.Set(k, v)
		}
		if got := From(r); got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}
}
