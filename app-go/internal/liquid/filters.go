package liquid

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// filterInput is a filter's input value and arguments.
type filterInput struct {
	v     value
	args  []value
	named map[string]value
	pos   int
}

// arg returns argument i, or def when it was not given.
func (in filterInput) arg(i int, def value) value {
	if i < len(in.args) {
		return in.args[i]
	}
	return def
}

func (in filterInput) str() string { return toString(in.v) }

type filterFunc func(r *renderer, in filterInput) (value, error)

// filters are the filters of spec §8.2. A filter that is not here renders as an empty
// string.
var filters map[string]filterFunc

func init() {
	filters = map[string]filterFunc{
		// math
		"plus":       arith(addInt, func(a, b float64) float64 { return a + b }),
		"minus":      arith(subInt, func(a, b float64) float64 { return a - b }),
		"times":      arith(mulInt, func(a, b float64) float64 { return a * b }),
		"divided_by": dividedBy,
		"modulo":     modulo,
		"abs":        unaryNumber(absInt, math.Abs),
		"ceil":       rounding(math.Ceil),
		"floor":      rounding(math.Floor),
		"round":      round,
		"at_least":   bound(func(c int) bool { return c < 0 }),
		"at_most":    bound(func(c int) bool { return c > 0 }),

		// string
		"append":         str2(func(s, a string) string { return s + a }),
		"prepend":        str2(func(s, a string) string { return a + s }),
		"upcase":         str1(strings.ToUpper),
		"downcase":       str1(strings.ToLower),
		"capitalize":     str1(capitalize),
		"strip":          str1(strings.TrimSpace),
		"lstrip":         str1(func(s string) string { return strings.TrimLeftFunc(s, unicode.IsSpace) }),
		"rstrip":         str1(func(s string) string { return strings.TrimRightFunc(s, unicode.IsSpace) }),
		"replace":        replace(-1, false),
		"replace_first":  replace(1, false),
		"remove":         replace(-1, true),
		"remove_first":   replace(1, true),
		"split":          split,
		"truncate":       truncate,
		"truncatewords":  truncateWords,
		"slice":          slice,
		"escape":         str1(escapeHTML),
		"url_encode":     str1(urlEncode),
		"url_decode":     urlDecode,
		"newline_to_br":  str1(func(s string) string { return newlines.Replace(s) }),
		"strip_newlines": str1(func(s string) string { return stripNewlines.Replace(s) }),

		// array
		"join":    join,
		"first":   firstLast(true),
		"last":    firstLast(false),
		"size":    size,
		"reverse": reverse,
		"sort":    sortFilter,
		"uniq":    uniq,
		"map":     mapFilter,
		"compact": compact,
		"concat":  concat,

		// other
		"default": defaultFilter,
		"json":    jsonFilter,
		"date":    dateFilter,
	}
}

// ── math ────────────────────────────────────────────────────────────────────────

// arith applies an operation to the input and the first argument, as numbers: on
// integers when both are integers and the result fits, on floats otherwise.
func arith(intOp func(a, b int64) (int64, bool), floatOp func(a, b float64) float64) filterFunc {
	return func(_ *renderer, in filterInput) (value, error) {
		a, b := toNumber(in.v), toNumber(in.arg(0, nil))
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				if n, ok := intOp(ai, bi); ok {
					return n, nil
				}
			}
		}
		return floatOp(toFloat(a), toFloat(b)), nil
	}
}

func addInt(a, b int64) (int64, bool) {
	s := a + b
	return s, (s > a) == (b > 0)
}

func subInt(a, b int64) (int64, bool) {
	s := a - b
	return s, (s < a) == (b > 0)
}

func mulInt(a, b int64) (int64, bool) {
	if a == 0 || b == 0 {
		return 0, true
	}
	p := a * b
	return p, p/b == a && !(a == -1 && b == math.MinInt64) && !(b == -1 && a == math.MinInt64)
}

func absInt(a int64) (int64, bool) {
	if a < 0 {
		return -a, a != math.MinInt64
	}
	return a, true
}

// dividedBy divides; two integers give the integer quotient, rounded down.
func dividedBy(r *renderer, in filterInput) (value, error) {
	a, b := toNumber(in.v), toNumber(in.arg(0, nil))
	if toFloat(b) == 0 {
		return nil, r.errAt(in.pos, "divided_by: division by zero")
	}
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok && !(ai == math.MinInt64 && bi == -1) {
			q := ai / bi
			if ai%bi != 0 && (ai < 0) != (bi < 0) {
				q--
			}
			return q, nil
		}
	}
	return toFloat(a) / toFloat(b), nil
}

// modulo is the remainder, with the sign of the input (as % in JavaScript).
func modulo(r *renderer, in filterInput) (value, error) {
	a, b := toNumber(in.v), toNumber(in.arg(0, nil))
	if toFloat(b) == 0 {
		return nil, r.errAt(in.pos, "modulo: division by zero")
	}
	if ai, ok := a.(int64); ok {
		if bi, ok := b.(int64); ok {
			if bi == -1 {
				return int64(0), nil
			}
			return ai % bi, nil
		}
	}
	return math.Mod(toFloat(a), toFloat(b)), nil
}

func unaryNumber(intOp func(int64) (int64, bool), floatOp func(float64) float64) filterFunc {
	return func(_ *renderer, in filterInput) (value, error) {
		n := toNumber(in.v)
		if i, ok := n.(int64); ok {
			if out, ok := intOp(i); ok {
				return out, nil
			}
		}
		return floatOp(toFloat(n)), nil
	}
}

// floatToInt makes a whole float an integer when it fits.
func floatToInt(f float64) value {
	if f >= -(1<<63) && f < 1<<63 {
		return int64(f)
	}
	return f
}

func rounding(op func(float64) float64) filterFunc {
	return func(_ *renderer, in filterInput) (value, error) {
		n := toNumber(in.v)
		if i, ok := n.(int64); ok {
			return i, nil
		}
		return floatToInt(op(n.(float64))), nil
	}
}

// round rounds to an integer, or to as many decimals as the argument says.
func round(_ *renderer, in filterInput) (value, error) {
	n := toNumber(in.v)
	digits := toInt(in.arg(0, int64(0)))
	if i, ok := n.(int64); ok {
		return i, nil
	}
	f := n.(float64)
	if digits <= 0 {
		return floatToInt(math.Round(f)), nil
	}
	if digits > 15 {
		return f, nil
	}
	p := math.Pow(10, float64(digits))
	return math.Round(f*p) / p, nil
}

// bound is at_least and at_most: the argument when replace(compare(input, arg)) holds.
func bound(replace func(c int) bool) filterFunc {
	return func(_ *renderer, in filterInput) (value, error) {
		a, b := toNumber(in.v), toNumber(in.arg(0, nil))
		if c, ok := order(a, b); ok && replace(c) {
			return b, nil
		}
		return a, nil
	}
}

// ── string ──────────────────────────────────────────────────────────────────────

func str1(f func(string) string) filterFunc {
	return func(_ *renderer, in filterInput) (value, error) { return f(in.str()), nil }
}

func str2(f func(s, a string) string) filterFunc {
	return func(r *renderer, in filterInput) (value, error) {
		s, a := in.str(), toString(in.arg(0, nil))
		if err := r.reserve(len(s)+len(a), in.pos); err != nil {
			return nil, err
		}
		return f(s, a), nil
	}
}

func capitalize(s string) string {
	c, n := utf8.DecodeRuneInString(s)
	if n == 0 {
		return s
	}
	return string(unicode.ToUpper(c)) + strings.ToLower(s[n:])
}

// replace is replace (n = -1), replace_first (n = 1), and remove and remove_first
// (remove: the replacement is "").
func replace(n int, remove bool) filterFunc {
	return func(r *renderer, in filterInput) (value, error) {
		s, old := in.str(), toString(in.arg(0, nil))
		repl := ""
		if !remove {
			repl = toString(in.arg(1, nil))
		}
		count := strings.Count(s, old)
		if n >= 0 {
			count = min(count, n)
		}
		if err := r.reserve(len(s)+count*len(repl), in.pos); err != nil {
			return nil, err
		}
		return strings.Replace(s, old, repl, n), nil
	}
}

// split splits on the argument (on every character when it is ""), and drops empty
// strings at the end, as LiquidJS does.
func split(r *renderer, in filterInput) (value, error) {
	s, sep := in.str(), toString(in.arg(0, nil))
	n := strings.Count(s, sep) + 1
	if sep == "" {
		n = utf8.RuneCountInString(s)
	}
	if err := r.step(n, in.pos); err != nil {
		return nil, err
	}
	parts := strings.Split(s, sep)
	for len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	out := make([]value, len(parts))
	for i, p := range parts {
		out[i] = p
	}
	return out, nil
}

// truncate shortens to the length (50) in characters, the ellipsis ("...") included.
func truncate(_ *renderer, in filterInput) (value, error) {
	s := []rune(in.str())
	n := toInt(in.arg(0, int64(50)))
	ellipsis := toString(in.arg(1, "..."))
	if int64(len(s)) <= n {
		return string(s), nil
	}
	keep := max(n-int64(utf8.RuneCountInString(ellipsis)), 0)
	return string(s[:keep]) + ellipsis, nil
}

// truncatewords keeps the first words (15, at least 1), joined by single spaces, and
// adds the ellipsis ("...") when there were more.
func truncateWords(_ *renderer, in filterInput) (value, error) {
	s := in.str()
	n := max(toInt(in.arg(0, int64(15))), 1)
	ellipsis := toString(in.arg(1, "..."))
	words := strings.Fields(s)
	if int64(len(words)) <= n {
		return s, nil
	}
	return strings.Join(words[:n], " ") + ellipsis, nil
}

// slice returns length (1) characters or elements from start; a negative start counts
// from the end.
func slice(r *renderer, in filterInput) (value, error) {
	start, length := toInt(in.arg(0, int64(0))), toInt(in.arg(1, int64(1)))
	cut := func(n int64) (int64, int64) {
		if start < 0 {
			start = max(start+n, 0)
		}
		from := min(start, n)
		return from, from + min(max(length, 0), n-from)
	}
	switch v := in.v.(type) {
	case []value, rangeValue:
		arr, err := r.array(v, in.pos)
		if err != nil {
			return nil, err
		}
		from, to := cut(int64(len(arr)))
		return slices.Clone(arr[from:to]), nil
	}
	s := []rune(in.str())
	from, to := cut(int64(len(s)))
	return string(s[from:to]), nil
}

var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&#34;", "'", "&#39;")

func escapeHTML(s string) string { return htmlEscaper.Replace(s) }

// urlEncode is JavaScript's encodeURIComponent with spaces as "+", as in LiquidJS.
func urlEncode(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9', strings.IndexByte("-_.!~*'()", c) >= 0:
			b.WriteByte(c)
		case c == ' ':
			b.WriteByte('+')
		default:
			fmt.Fprintf(&b, "%%%02X", c)
		}
	}
	return b.String()
}

func urlDecode(r *renderer, in filterInput) (value, error) {
	s, err := url.QueryUnescape(in.str())
	if err != nil {
		return nil, r.errAt(in.pos, "url_decode: "+strings.Replace(err.Error(), "URL ", "", 1))
	}
	return s, nil
}

var (
	newlines      = strings.NewReplacer("\r\n", "<br />\n", "\n", "<br />\n")
	stripNewlines = strings.NewReplacer("\r\n", "", "\n", "")
)

// ── array ───────────────────────────────────────────────────────────────────────

func join(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	sep := toString(in.arg(0, " "))
	parts := make([]string, len(arr))
	n := len(sep) * len(arr)
	for i, e := range arr {
		parts[i] = toString(e)
		n += len(parts[i])
	}
	if err := r.reserve(n, in.pos); err != nil {
		return nil, err
	}
	return strings.Join(parts, sep), nil
}

// firstLast is first and last of an array or range, or the first and last character
// of a string.
func firstLast(first bool) filterFunc {
	return func(r *renderer, in filterInput) (value, error) {
		switch v := in.v.(type) {
		case string:
			if v == "" {
				return nil, nil
			}
			if first {
				c, _ := utf8.DecodeRuneInString(v)
				return string(c), nil
			}
			c, _ := utf8.DecodeLastRuneInString(v)
			return string(c), nil
		case []value, rangeValue:
			key := "last"
			if first {
				key = "first"
			}
			return property(v, key), nil
		}
		return nil, nil
	}
}

func size(_ *renderer, in filterInput) (value, error) {
	switch v := in.v.(type) {
	case map[string]value:
		return int64(len(v)), nil
	case []value, rangeValue, string:
		return property(v, "size"), nil
	}
	return int64(0), nil
}

func reverse(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	out := slices.Clone(arr)
	slices.Reverse(out)
	return out, nil
}

// sortFilter sorts numbers by value and strings byte by byte, numbers before strings
// and nil last; with an argument, by that property of each element.
func sortFilter(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	key := func(v value) value { return v }
	if len(in.args) > 0 {
		k := in.args[0]
		key = func(v value) value { return property(v, k) }
	}
	rank := func(v value) int {
		switch {
		case v == nil:
			return 3
		case isNumber(v):
			return 0
		}
		if _, ok := v.(string); ok {
			return 1
		}
		return 2
	}
	out := slices.Clone(arr)
	slices.SortStableFunc(out, func(a, b value) int {
		ka, kb := key(a), key(b)
		if ra, rb := rank(ka), rank(kb); ra != rb {
			return ra - rb
		}
		c, _ := order(ka, kb)
		return c
	})
	return out, r.step(len(out), in.pos)
}

// uniq drops later elements equal to an earlier one.
func uniq(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var out []value
	for _, e := range arr {
		k := uniqKey(e)
		if !seen[k] {
			seen[k] = true
			out = append(out, e)
		}
	}
	return out, r.step(len(arr), in.pos)
}

// uniqKey is the same for values that are equal.
func uniqKey(v value) string {
	switch v := v.(type) {
	case nil:
		return "z"
	case bool:
		return "b" + strconv.FormatBool(v)
	case int64:
		return "n" + strconv.FormatInt(v, 10)
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<63 {
			return "n" + strconv.FormatInt(int64(v), 10)
		}
		return "f" + strconv.FormatFloat(v, 'g', -1, 64)
	case string:
		return "s" + v
	}
	b, _ := json.Marshal(jsonable(v))
	return fmt.Sprintf("%T%s", v, b)
}

// mapFilter is the property named by the argument of each element.
func mapFilter(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	k := in.arg(0, nil)
	out := make([]value, len(arr))
	for i, e := range arr {
		out[i] = property(e, k)
	}
	return out, r.step(len(out), in.pos)
}

func compact(r *renderer, in filterInput) (value, error) {
	arr, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	out := make([]value, 0, len(arr))
	for _, e := range arr {
		if e != nil {
			out = append(out, e)
		}
	}
	return out, nil
}

func concat(r *renderer, in filterInput) (value, error) {
	a, err := r.array(in.v, in.pos)
	if err != nil {
		return nil, err
	}
	b, err := r.array(in.arg(0, nil), in.pos)
	if err != nil {
		return nil, err
	}
	if err := r.step(len(a)+len(b), in.pos); err != nil {
		return nil, err
	}
	return slices.Concat(a, b), nil
}

// ── other ───────────────────────────────────────────────────────────────────────

// defaultFilter returns the argument for nil, false, "" and empty arrays; false stays
// when allow_false is true.
func defaultFilter(_ *renderer, in filterInput) (value, error) {
	if in.v == false && truthy(in.named["allow_false"]) {
		return false, nil
	}
	if in.v == nil || in.v == false || isEmpty(in.v) {
		return in.arg(0, nil), nil
	}
	return in.v, nil
}

// jsonFilter writes the value as JSON, indented when the argument is a number of spaces.
func jsonFilter(r *renderer, in filterInput) (value, error) {
	v := in.v
	if rv, ok := v.(rangeValue); ok {
		arr, err := r.array(rv, in.pos)
		if err != nil {
			return nil, err
		}
		v = arr
	}
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	enc.SetEscapeHTML(false)
	if n := toInt(in.arg(0, int64(0))); n > 0 {
		enc.SetIndent("", strings.Repeat(" ", int(min(n, 10))))
	}
	if err := enc.Encode(jsonable(v)); err != nil {
		return nil, r.errAt(in.pos, "json: "+err.Error())
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}

// jsonable replaces what encoding/json cannot write: ranges by their text, empty and
// blank by "".
func jsonable(v value) value {
	switch v := v.(type) {
	case special:
		return ""
	case rangeValue:
		return toString(v)
	case loopValue:
		return v.toMap()
	case []value:
		out := make([]value, len(v))
		for i, e := range v {
			out[i] = jsonable(e)
		}
		return out
	case map[string]value:
		out := make(map[string]value, len(v))
		for k, e := range v {
			out[k] = jsonable(e)
		}
		return out
	}
	return v
}
