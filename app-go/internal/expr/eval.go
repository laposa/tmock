package expr

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"math"
	"math/big"
	"mime"
	"net/url"
	"strconv"
	"strings"
)

// Request is the $request namespace (spec §7.4).
type Request struct {
	Method  string
	Path    string            // the decoded request path, /api/proxy/… included
	Query   map[string]string // the first value of each query parameter
	Headers map[string]string // lower-case names; several values joined with ", "
	// Body is the parsed body (see BodyValue): nil, bool, a number, string, []any or
	// map[string]any. Numbers are *big.Float (see BodyValue); float64 and json.Number
	// are accepted too.
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
	if n, ok := toNum(v); ok {
		return n.Sign() != 0
	}
	switch v := v.(type) {
	case nil:
		return false
	case bool:
		return v
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
	if x, ok := toNum(a); ok {
		y, ok := toNum(b)
		if !ok {
			return false
		}
		c = x.Cmp(y)
	} else {
		switch x := a.(type) {
		case string:
			y, ok := b.(string)
			if !ok {
				return false
			}
			c = strings.Compare(x, y)
		default:
			return false
		}
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

// equal compares two JSON values by type and value: "1" is not 1, and numbers compare
// numerically (1 is 1.0), also inside arrays and objects.
func equal(a, b any) bool {
	if x, ok := toNum(a); ok {
		y, ok := toNum(b)
		return ok && x.Cmp(y) == 0
	}
	switch x := a.(type) {
	case nil:
		return b == nil
	case bool:
		y, ok := b.(bool)
		return ok && x == y
	case string:
		y, ok := b.(string)
		return ok && x == y
	case []any:
		y, ok := b.([]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for i := range x {
			if !equal(x[i], y[i]) {
				return false
			}
		}
		return true
	case map[string]any:
		y, ok := b.(map[string]any)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if w, found := y[k]; !found || !equal(v, w) {
				return false
			}
		}
		return true
	case map[string]string:
		y, ok := b.(map[string]string)
		if !ok || len(x) != len(y) {
			return false
		}
		for k, v := range x {
			if w, found := y[k]; !found || v != w {
				return false
			}
		}
		return true
	}
	return false
}

// numPrec is the precision of numbers, in bits: integers of up to about 150 digits are
// exact, so large ids compare correctly (float64 is exact only up to 2^53).
const numPrec = 512

// maxExactLen is the longest number text parsed exactly. Longer ones are parsed as
// float64, which takes linear time: exact parsing of a huge number is quadratic, and a
// body can hold a number of 10 MB.
const maxExactLen = 200

// parseNumber parses the text of a JSON number (validNumber).
func parseNumber(raw string) (*big.Float, bool) {
	if len(raw) <= maxExactLen {
		if f, _, err := big.ParseFloat(raw, 10, numPrec, big.ToNearestEven); err == nil {
			return f, true
		}
	}
	// Too long, or an exponent too large for big.Float's parser: float64, where an
	// out-of-range value is ±Inf or 0.
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil && !errors.Is(err, strconv.ErrRange) {
		return nil, false
	}
	return new(big.Float).SetPrec(numPrec).SetFloat64(f), true
}

// validNumber reports whether s is -?digits(.digits)?([eE][+-]?digits)?: a JSON number,
// except that leading zeros are allowed.
func validNumber(s string) bool {
	i := 0
	digits := func() bool {
		start := i
		for i < len(s) && isDigit(s[i]) {
			i++
		}
		return i > start
	}
	if i < len(s) && s[i] == '-' {
		i++
	}
	if !digits() {
		return false
	}
	if i < len(s) && s[i] == '.' {
		i++
		if !digits() {
			return false
		}
	}
	if i < len(s) && (s[i] == 'e' || s[i] == 'E') {
		i++
		if i < len(s) && (s[i] == '+' || s[i] == '-') {
			i++
		}
		if !digits() {
			return false
		}
	}
	return i == len(s)
}

// toNum returns v as a number, if it is one.
func toNum(v any) (*big.Float, bool) {
	switch n := v.(type) {
	case *big.Float:
		return n, n != nil
	case float64:
		if math.IsNaN(n) {
			return nil, false
		}
		return new(big.Float).SetPrec(numPrec).SetFloat64(n), true
	case json.Number:
		if !validNumber(string(n)) {
			return nil, false
		}
		return parseNumber(string(n))
	}
	return nil, false
}

// exactNumbers replaces the json.Number values of a decoded JSON value with *big.Float.
func exactNumbers(v any) any {
	switch x := v.(type) {
	case json.Number:
		if n, ok := toNum(x); ok {
			return n
		}
		return nil
	case []any:
		for i := range x {
			x[i] = exactNumbers(x[i])
		}
	case map[string]any:
		for k := range x {
			x[k] = exactNumbers(x[k])
		}
	}
	return v
}

// BodyValue parses a request body for $request.body (spec §7.4): JSON when the content
// type is application/json, ends in +json, or is missing; the form map (first values)
// for application/x-www-form-urlencoded; otherwise, and for a body that does not parse,
// nil. JSON numbers are *big.Float.
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
		// Numbers are decoded exactly (json.Number, then big.Float), so that a large id
		// is not rounded to the nearest float64.
		dec := json.NewDecoder(bytes.NewReader(body))
		dec.UseNumber()
		var v any
		if err := dec.Decode(&v); err != nil {
			return nil
		}
		if _, err := dec.Token(); err != io.EOF {
			return nil // something after the value
		}
		return exactNumbers(v)
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
