package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/laposa/tmock/app-go/internal/catalog"
)

// mockedParts lists what scenario s changes in a response, for the log (spec §7.9):
// "status", "headers" and "body", in that order, only those it defines.
func mockedParts(s *catalog.Scenario) []string {
	var out []string
	if s.ResponseCode != nil {
		out = append(out, "status")
	}
	if len(s.ResponseHeaders) > 0 {
		out = append(out, "headers")
	}
	if s.ResponseBody != nil {
		out = append(out, "body")
	}
	return out
}

// responseBody is the body a scenario writes. Templates are rendered from I12; until then
// the body is used verbatim, marker included.
func responseBody(s *catalog.Scenario) []byte {
	return []byte(*s.ResponseBody)
}

// skipsProxy reports whether the scenario answers without contacting the upstream
// (spec §7.6). skipProxy implies a body; a scenario without one is proxied.
func skipsProxy(s *catalog.Scenario) bool {
	return s != nil && s.SkipProxy && s.ResponseBody != nil
}

// writeMock answers the request from the scenario alone (spec §7.6): its status (200 when
// unset), its headers (a null value is ignored), the content-type fallback, the
// x-tmock-* headers and the body.
func writeMock(w http.ResponseWriter, sel selection) {
	s := sel.scenario
	body := responseBody(s)
	h := w.Header()
	for name, v := range s.ResponseHeaders {
		if v != nil {
			h.Set(name, *v)
		}
	}
	setContentTypeFallback(h, body)
	setTmockHeaders(h, StatusMocked, sel)
	status := http.StatusOK
	if s.ResponseCode != nil {
		status = *s.ResponseCode
	}
	if !bodyAllowed(status) {
		// A scenario's own Content-Length or Transfer-Encoding must not survive either.
		h.Del("Content-Length")
		h.Del("Transfer-Encoding")
		w.WriteHeader(status)
		return
	}
	h.Set("Content-Length", strconv.Itoa(len(body)))
	w.WriteHeader(status)
	_, _ = w.Write(body)
}

// bodyAllowed reports whether a response with status may have a body (RFC 9110: not
// 1xx, 204 or 304).
func bodyAllowed(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusNotModified
}

// modifyUpstream changes the upstream response as the scenario says (spec §7.8):
//  1. the scenario's headers over the upstream's (null deletes the header);
//  2. its status, if set;
//  3. its body, if set, instead of the upstream's, without ETag, Content-Encoding and
//     Content-Length;
//  4. if tmock wrote the body and there is no Content-Type, application/json for valid
//     JSON, otherwise text/plain;
//  5. the x-tmock-* headers.
//
// Without a replaced body the upstream's body is streamed untouched, so it is not looked
// at for the fallback either. A status that allows no body (204, 304) drops whichever
// body there is, since writing one would abort the response.
//
// An upgrade (101 Switching Protocols) gets only the headers: its body is the upgraded
// connection, and a new status would break the upgrade.
func modifyUpstream(resp *http.Response, sel selection) {
	s := sel.scenario
	h := resp.Header
	if resp.StatusCode == http.StatusSwitchingProtocols {
		setTmockHeaders(h, StatusMocked, sel)
		return
	}
	for name, v := range s.ResponseHeaders {
		if v == nil {
			h.Del(name)
		} else {
			h.Set(name, *v)
		}
	}
	if s.ResponseCode != nil {
		resp.StatusCode = *s.ResponseCode
		resp.Status = strconv.Itoa(resp.StatusCode) + " " + http.StatusText(resp.StatusCode)
	}
	if s.ResponseBody != nil {
		body := responseBody(s)
		_ = resp.Body.Close()
		replaceBody(resp, body)
		h.Del("Etag")
		h.Del("Content-Encoding")
		setContentTypeFallback(h, body)
	}
	if !bodyAllowed(resp.StatusCode) {
		_ = resp.Body.Close()
		replaceBody(resp, nil)
	}
	setTmockHeaders(h, StatusMocked, sel)
}

// replaceBody gives resp a fixed body (none when nil) in place of the upstream's,
// without the upstream's framing: Content-Length, Transfer-Encoding and trailers.
func replaceBody(resp *http.Response, body []byte) {
	if body == nil {
		resp.Body = http.NoBody
	} else {
		resp.Body = io.NopCloser(bytes.NewReader(body))
	}
	resp.ContentLength = int64(len(body))
	resp.TransferEncoding = nil
	resp.Uncompressed = false
	resp.Trailer = nil
	resp.Header.Del("Content-Length")
	resp.Header.Del("Transfer-Encoding")
	resp.Header.Del("Trailer")
}

// setContentTypeFallback sets a Content-Type when there is none (spec §7.8 step 4).
func setContentTypeFallback(h http.Header, body []byte) {
	if h.Get("Content-Type") != "" {
		return
	}
	if json.Valid(body) {
		h.Set("Content-Type", "application/json")
	} else {
		h.Set("Content-Type", "text/plain; charset=utf-8")
	}
}

// setTmockHeaders sets x-tmock-status, and x-tmock-client and x-tmock-scenario when a
// client or a scenario matched, after removing every other x-tmock-* header: they only
// ever come from tmock (spec §7.7, §7.8), never from what the client, the upstream or a
// scenario's headers sent.
func setTmockHeaders(h http.Header, status string, sel selection) {
	for k := range h {
		if strings.HasPrefix(k, "X-Tmock-") {
			delete(h, k)
		}
	}
	h.Set(HeaderStatus, status)
	if sel.client != nil {
		h.Set(HeaderClient, headerSafe(sel.client.Name))
	}
	if sel.scenario != nil {
		h.Set(HeaderScenario, headerSafe(sel.scenario.Name))
	}
}

// headerSafe replaces the control characters that a header value cannot hold (a name may
// contain any text) with spaces, so a client named "a\nb" cannot fail the request.
func headerSafe(s string) string {
	return strings.Map(func(r rune) rune {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return ' '
		}
		return r
	}, s)
}
