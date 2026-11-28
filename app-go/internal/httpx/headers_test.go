package httpx

import "testing"

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
