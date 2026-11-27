package expr

import (
	"bytes"
	"encoding/json"
	"mime"
	"net/url"
	"reflect"
	"strings"
)

// Request is the $request namespace (spec §7.4).
type Request struct {
	Method  string
	Path    string            // the decoded request path, /api/proxy/… included
	Query   map[string]string // the first value of each query parameter
	Headers map[string]string // lower-case names; several values joined with ", "
	// Body is the parsed body (see BodyValue): nil, bool, float64, string, []any or
	// map[string]any.
	Body any
	IP   string // the client IP (spec §7.3), "" when unknown
}

// Eval reports whether the expression is true for req. It cannot fail: a missing path is
// null, and a comparison of values that cannot be ordered is false.
func (e *Expr) Eval(req *Request) bool {
	return truthy(eval(e.root, req))
}

func eval(n node, req *Request) any {
	switch n.kind {
	case nodeLiteral:
		return n.value
	case nodeRef:
		return resolve(req, n.path)
	case nodeNot:
		return !truthy(eval(n.kids[0], req))
	case nodeAnd:
		for _, k := range n.kids {
			if !truthy(eval(k, req)) {
				return false
			}
		}
		return true
	case nodeOr:
		for _, k := range n.kids {
			if truthy(eval(k, req)) {
				return true
			}
		}
		return false
	case nodeCmp:
		return compare(n.op, eval(n.kids[0], req), eval(n.kids[1], req))
	}
	return nil
}

// resolve follows path from $request. Anything missing, or of the wrong kind for the
// segment (a name on an array, an index on an object), is nil.
func resolve(req *Request, path []any) any {
	if req == nil {
		return nil
	}
	var cur any
	if len(path) == 0 {
		return map[string]any{"method": req.Method, "path": req.Path, "query": req.Query,
			"headers": req.Headers, "body": req.Body, "ip": req.IP}
	}
	switch path[0] {
	case "method":
		cur = req.Method
	case "path":
		cur = req.Path
	case "query":
		cur = req.Query
	case "headers":
		cur = req.Headers
	case "body":
		cur = req.Body
	case "ip":
		cur = req.IP
	default:
		return nil
	}
	for _, seg := range path[1:] {
		switch v := cur.(type) {
		case map[string]string:
			key, isKey := seg.(string)
			s, found := v[key]
			if !isKey || !found {
				return nil
			}
			cur = s
		case map[string]any:
			key, ok := seg.(string)
			if !ok {
				return nil
			}
			cur = v[key]
		case []any:
			i, ok := seg.(int)
			if !ok || i >= len(v) {
				return nil
			}
			cur = v[i]
		default:
			return nil
		}
	}
	if m, ok := cur.(map[string]string); ok && m == nil {
		return nil
	}
	return cur
}

// truthy: null, false, 0 and "" are false; everything else is true.
func truthy(v any) bool {
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
	case float64:
		return v != 0
	case string:
		return v != ""
	}
	return true
}

func compare(op string, a, b any) bool {
	switch op {
	case "==":
		return equal(a, b)
	case "!=":
		return !equal(a, b)
	}
	var c int
	switch x := a.(type) {
	case float64:
		y, ok := b.(float64)
		if !ok {
			return false
		}
		switch {
		case x < y:
			c = -1
		case x > y:
			c = 1
		}
	case string:
		y, ok := b.(string)
		if !ok {
			return false
		}
		c = strings.Compare(x, y)
	default:
		return false
	}
	switch op {
	case "<":
		return c < 0
	case "<=":
		return c <= 0
	case ">":
		return c > 0
	case ">=":
		return c >= 0
	}
	return false
}

// equal compares two JSON values by type and value: "1" is not 1.
func equal(a, b any) bool {
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case float64:
		y, ok := b.(float64)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	}
	return reflect.DeepEqual(a, b)
}

// BodyValue parses a request body for $request.body (spec §7.4): JSON when the content
// type is application/json, ends in +json, or is missing; the form map (first values)
// for application/x-www-form-urlencoded; otherwise, and for a body that does not parse,
// nil.
func BodyValue(contentType string, body []byte) any {
	if len(bytes.TrimSpace(body)) == 0 {
		return nil
	}
	mt := ""
	if contentType != "" {
		var err error
		if mt, _, err = mime.ParseMediaType(contentType); err != nil {
			return nil
		}
	}
	switch {
	case mt == "", mt == "application/json", strings.HasSuffix(mt, "+json"):
		var v any
		if err := json.Unmarshal(body, &v); err != nil {
			return nil
		}
		return v
	case mt == "application/x-www-form-urlencoded":
		vals, err := url.ParseQuery(string(body))
		if err != nil {
			return nil
		}
		out := make(map[string]any, len(vals))
		for k, v := range vals {
			out[k] = v[0]
		}
		return out
	}
	return nil
}

// FirstValues turns query parameters into the map $request.query holds.
func FirstValues(v url.Values) map[string]string {
	out := make(map[string]string, len(v))
	for k, vs := range v {
		if len(vs) > 0 {
			out[k] = vs[0]
		}
	}
	return out
}
