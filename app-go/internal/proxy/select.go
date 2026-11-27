package proxy

import (
	"bytes"
	"errors"
	"io"
	"net/http"
	"net/netip"
	"strings"

	"github.com/laposa/tmock/app-go/internal/catalog"
	"github.com/laposa/tmock/app-go/internal/clientip"
	"github.com/laposa/tmock/app-go/internal/condition"
	"github.com/laposa/tmock/app-go/internal/expr"
	"github.com/laposa/tmock/app-go/internal/httpx"
)

// selection is the outcome of client matching and scenario selection for one request.
type selection struct {
	client   *catalog.Client   // the reported client; nil when no client matched
	scenario *catalog.Scenario // the winning scenario; nil when none applies
}

// matchingClients returns the enabled clients whose condition is true for r, in id order
// (spec §7.3).
func matchingClients(clients []catalog.Client, r *http.Request) []*catalog.Client {
	if len(clients) == 0 {
		return nil
	}
	cr := condition.Request{Header: r.Header, Host: r.Host}
	if ip, err := netip.ParseAddr(clientip.From(r)); err == nil {
		cr.IP = ip.Unmap()
	}
	var out []*catalog.Client
	for i := range clients {
		if clients[i].Match.Match(cr) {
			out = append(out, &clients[i])
		}
	}
	return out
}

// requestPath is the part of the path after /api/proxy/{service}/, decoded, without
// leading or trailing "/" (spec §7.5 step 2). The query is not part of it.
func requestPath(r *http.Request) string {
	return strings.Trim(r.PathValue("rest"), "/")
}

// conditionEnv provides the $request namespace that request conditions are evaluated
// against. *requestEnv is the proxy's.
type conditionEnv interface {
	// Request returns the namespace; with needBody, its body is read and parsed. It
	// fails only when the body cannot be read (errBodyTooLarge, or the client went away).
	Request(needBody bool) (*expr.Request, error)
}

// selectScenario applies spec §7.5 to the scenarios of svc for the matching clients:
// the first scenario, in name order, that one of the clients has assigned and whose
// method, path and request-condition filters fit the request wins. The reported client
// is the first matching client that has the winner assigned, or the first matching client
// when no scenario wins.
//
// A request condition is evaluated last, only for a scenario that passes the other
// filters, so the body is read only when such a candidate's condition needs it (spec
// §7.7). The error is env's.
func selectScenario(svc *catalog.Service, clients []*catalog.Client, method, reqPath string, env conditionEnv) (selection, error) {
	if len(clients) == 0 {
		return selection{}, nil
	}
	for i := range svc.Scenarios {
		s := &svc.Scenarios[i]
		var owner *catalog.Client
		for _, c := range clients {
			if c.Scenarios[s.ID] {
				owner = c
				break
			}
		}
		switch {
		case owner == nil:
		case s.RequestPath != nil && !strings.EqualFold(*s.RequestPath, reqPath):
		case s.RequestMethod != nil && *s.RequestMethod != method:
		case s.Condition != nil:
			req, err := env.Request(s.Condition.UsesBody())
			if err != nil {
				return selection{client: clients[0]}, err
			}
			if s.Condition.Eval(req) {
				return selection{client: owner, scenario: s}, nil
			}
		default:
			return selection{client: owner, scenario: s}, nil
		}
	}
	return selection{client: clients[0]}, nil
}

// errBodyTooLarge is returned when a request condition needs a body larger than
// httpx.MaxBodyBytes (spec §7.7: 413).
var errBodyTooLarge = errors.New("request body too large")

// requestEnv builds the $request namespace of one request lazily: the headers and query
// on the first condition, the body only for a condition that needs it. Reading the body
// buffers it and puts it back, so the upstream still receives it unchanged.
type requestEnv struct {
	r        *http.Request
	req      *expr.Request
	bodyRead bool
	err      error
}

func (e *requestEnv) Request(needBody bool) (*expr.Request, error) {
	if e.req == nil {
		r := e.r
		headers := make(map[string]string, len(r.Header)+1)
		for k, v := range r.Header {
			headers[strings.ToLower(k)] = strings.Join(v, ", ")
		}
		if r.Host != "" {
			headers["host"] = r.Host // Go's server keeps Host out of Header
		}
		e.req = &expr.Request{
			Method: r.Method, Path: r.URL.Path, Query: expr.FirstValues(r.URL.Query()),
			Headers: headers, IP: clientip.From(r),
		}
	}
	if needBody && !e.bodyRead {
		e.bodyRead = true
		body, err := bufferBody(e.r)
		if err != nil {
			e.err = err
		} else {
			e.req.Body = expr.BodyValue(e.r.Header.Get("Content-Type"), body)
		}
	}
	return e.req, e.err
}

// bufferBody reads r's body, up to httpx.MaxBodyBytes, and replaces it with the bytes
// read, so that it can still be forwarded.
func bufferBody(r *http.Request) ([]byte, error) {
	if r.Body == nil || r.Body == http.NoBody {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, httpx.MaxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(body) > httpx.MaxBodyBytes {
		return nil, errBodyTooLarge
	}
	_ = r.Body.Close()
	// The upstream gets the body with a Content-Length, as it is known now.
	r.ContentLength = int64(len(body))
	r.TransferEncoding = nil
	if len(body) == 0 {
		r.Body = http.NoBody
	} else {
		r.Body = io.NopCloser(bytes.NewReader(body))
	}
	return body, nil
}
