package httpx

import (
	"net/http"
	"strings"
)

// CSP is the Content-Security-Policy of API and UI responses (spec §9.4).
const CSP = "default-src 'self'; base-uri 'self'; font-src 'self' data:; form-action 'self'; " +
	"frame-ancestors 'self'; img-src 'self' data:; object-src 'none'; script-src 'self'; " +
	"script-src-attr 'none'; style-src 'self'"

// SecurityHeaders sets the headers of spec §9.4 on every response of next. Production adds
// upgrade-insecure-requests to the CSP and Strict-Transport-Security. The headers are set
// before next runs, so a handler can still replace one, as the swagger page does with its
// relaxed CSP. Proxy responses must not go through this middleware.
func SecurityHeaders(production bool, next http.Handler) http.Handler {
	csp := CSP
	if production {
		csp += "; upgrade-insecure-requests"
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Content-Security-Policy", csp)
		h.Set("Cross-Origin-Opener-Policy", "same-origin")
		h.Set("Cross-Origin-Resource-Policy", "same-origin")
		h.Set("Referrer-Policy", "no-referrer")
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "SAMEORIGIN")
		if production {
			h.Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(w, r)
	})
}

// CrossOrigin wraps next in http.CrossOriginProtection (spec §9.3): unsafe cross-origin
// browser requests (judged by Sec-Fetch-Site and Origin) get a 403 in the §6.2 format;
// requests without those headers, such as curl and test automation, pass.
func CrossOrigin(next http.Handler) http.Handler {
	p := http.NewCrossOriginProtection()
	p.SetDenyHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		WriteError(w, http.StatusForbidden, "Cross-origin request rejected", nil)
	}))
	return p.Handler(next)
}

// proxyMethods are the methods a preflight is told the proxy allows (spec §9.5).
const proxyMethods = "GET,HEAD,PUT,PATCH,POST,DELETE"

// ProxyCORS is the proxy's CORS middleware (spec §9.5). It reflects the request's Origin
// with credentials allowed, or only origins in allowed when that list is not empty
// (entries as config normalises them: lower-case scheme://host[:port]). Preflights are
// answered with 204 and never reach next. On other responses tmock's CORS headers are
// added when the response is written, and only where next (the upstream) did not set the
// header itself, so the upstream's own CORS headers win.
func ProxyCORS(allowed []string, next http.Handler) http.Handler {
	allow := map[string]bool{}
	for _, o := range allowed {
		allow[o] = true
	}
	permitted := func(origin string) bool {
		return origin != "" && (len(allow) == 0 || allow[strings.ToLower(origin)])
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if r.Method == http.MethodOptions && origin != "" && r.Header.Get("Access-Control-Request-Method") != "" {
			h := w.Header()
			h.Add("Vary", "Origin")
			h.Add("Vary", "Access-Control-Request-Method")
			h.Add("Vary", "Access-Control-Request-Headers")
			if permitted(origin) {
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Credentials", "true")
				h.Set("Access-Control-Allow-Methods", proxyMethods)
				if req := r.Header.Get("Access-Control-Request-Headers"); req != "" {
					h.Set("Access-Control-Allow-Headers", req)
				}
			}
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(&corsWriter{ResponseWriter: w, origin: origin, permitted: permitted(origin)}, r)
	})
}

// corsWriter adds the CORS headers just before the response header is sent.
type corsWriter struct {
	http.ResponseWriter
	origin    string
	permitted bool
	done      bool
}

func (c *corsWriter) addHeaders() {
	if c.done {
		return
	}
	c.done = true
	h := c.Header()
	if !containsToken(h.Values("Vary"), "Origin") {
		h.Add("Vary", "Origin")
	}
	if !c.permitted || h.Get("Access-Control-Allow-Origin") != "" {
		return // not allowed, or the upstream answers CORS itself
	}
	h.Set("Access-Control-Allow-Origin", c.origin)
	if h.Get("Access-Control-Allow-Credentials") == "" {
		h.Set("Access-Control-Allow-Credentials", "true")
	}
}

func (c *corsWriter) WriteHeader(code int) {
	if code >= 200 || code == http.StatusSwitchingProtocols {
		c.addHeaders()
	}
	c.ResponseWriter.WriteHeader(code)
}

func (c *corsWriter) Write(b []byte) (int, error) {
	c.addHeaders()
	return c.ResponseWriter.Write(b)
}

// Unwrap lets http.ResponseController reach the underlying writer (Flush, deadlines).
func (c *corsWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

// containsToken reports whether any of the comma-separated header values contains token,
// ignoring case.
func containsToken(values []string, token string) bool {
	for _, v := range values {
		for t := range strings.SplitSeq(v, ",") {
			if strings.EqualFold(strings.TrimSpace(t), token) {
				return true
			}
		}
	}
	return false
}

// JSONFallback serves mux, answering requests that match no route in the §6.2 format: an
// unknown path is a 404, a known path with the wrong method a 405 with an Allow header.
// http.ServeMux would answer both in plain text.
func JSONFallback(mux *http.ServeMux) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if _, pattern := mux.Handler(r); pattern != "" {
			mux.ServeHTTP(w, r)
			return
		}
		// Let the mux decide between 404 and 405 (and compute Allow), then write JSON.
		probe := &statusProbe{header: http.Header{}}
		mux.ServeHTTP(probe, r)
		if probe.code >= 300 && probe.code < 400 { // a redirect to the canonical path
			mux.ServeHTTP(w, r)
			return
		}
		if allow := probe.header.Get("Allow"); allow != "" {
			w.Header().Set("Allow", allow)
		}
		switch probe.code {
		case http.StatusMethodNotAllowed:
			WriteError(w, http.StatusMethodNotAllowed, "Method not allowed", nil)
		default:
			WriteError(w, http.StatusNotFound, "Not found", nil)
		}
	})
}

// statusProbe records the status and headers a handler writes, and discards the body.
type statusProbe struct {
	header http.Header
	code   int
}

func (p *statusProbe) Header() http.Header { return p.header }
func (p *statusProbe) WriteHeader(code int) {
	if p.code == 0 {
		p.code = code
	}
}
func (p *statusProbe) Write(b []byte) (int, error) {
	if p.code == 0 {
		p.code = http.StatusOK
	}
	return len(b), nil
}
