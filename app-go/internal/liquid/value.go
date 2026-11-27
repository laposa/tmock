package liquid

import (
	"encoding/json"
	"math"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// value is a template value: nil, bool, int64, float64, string, []value,
// map[string]value, rangeValue, loopValue (forloop), or the literals empty and blank.
type value = any

type special int

const (
	emptyValue special = iota + 1
	blankValue
)

// rangeValue is (lo..hi), kept lazy so that a for loop over a large range does not
// allocate it.
type rangeValue struct{ lo, hi int64 }

func (r rangeValue) len() int64 {
	if r.hi < r.lo {
		return 0
	}
	if n := r.hi - r.lo + 1; n > 0 {
		return n
	}
	return math.MaxInt64 // overflow: more than any budget allows anyway
}

// loopValue is forloop: index, index0, rindex, rindex0, first, last and length of
// iteration k of a loop of length iterations.
type loopValue struct{ k, length int64 }

func (l loopValue) get(key string) value {
	switch key {
	case "index":
		return l.k + 1
	case "index0":
		return l.k
	case "rindex":
		return l.length - l.k
	case "rindex0":
		return l.length - l.k - 1
	case "first":
		return l.k == 0
	case "last":
		return l.k == l.length-1
	case "length":
		return l.length
	}
	return nil
}

func (l loopValue) toMap() map[string]value {
	m := map[string]value{}
	for _, k := range []string{"index", "index0", "rindex", "rindex0", "first", "last", "length"} {
		m[k] = l.get(k)
	}
	return m
}

// truthy: only nil and false are false, as in Liquid ("" and 0 are true).
func truthy(v value) bool { return v != nil && v != false }

// toString is how a value is rendered.
func toString(v value) string {
	switch v := v.(type) {
	case nil, special:
		return ""
	case bool:
		return strconv.FormatBool(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return formatFloat(v)
	case string:
		return v
	case []value:
		var b strings.Builder
		for _, e := range v {
			b.WriteString(toString(e))
		}
		return b.String()
	case rangeValue:
		return strconv.FormatInt(v.lo, 10) + ".." + strconv.FormatInt(v.hi, 10)
	case map[string]value:
		b, _ := json.Marshal(v)
		return string(b)
	case loopValue:
		return toString(v.toMap())
	}
	return ""
}

// formatFloat writes the shortest form that reads back as f, without an exponent below
// 1e21 (as JavaScript does): 3.0 is "3", 0.1 is "0.1".
func formatFloat(f float64) string {
	switch {
	case math.IsNaN(f):
		return "NaN"
	case math.IsInf(f, 1):
		return "Infinity"
	case math.IsInf(f, -1):
		return "-Infinity"
	case math.Abs(f) < 1e21:
		return strconv.FormatFloat(f, 'f', -1, 64)
	}
	return strconv.FormatFloat(f, 'g', -1, 64)
}

// toNumber converts v to an int64 or a float64: numbers as they are, strings that hold a
// number to that number, everything else to 0.
func toNumber(v value) value {
	switch v := v.(type) {
	case int64, float64:
		return v
	case string:
		if n, ok := parseNumber(v); ok {
			return n
		}
	}
	return int64(0)
}

// parseNumber reads an integer or a decimal number, white space around it allowed.
func parseNumber(s string) (value, bool) {
	s = strings.TrimSpace(s)
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, true
	}
	if s == "" || strings.ContainsAny(s, "xXpP_") || strings.EqualFold(strings.TrimLeft(s, "+-"), "inf") ||
		strings.EqualFold(strings.TrimLeft(s, "+-"), "infinity") || strings.EqualFold(s, "nan") {
		return nil, false
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f, true
	}
	return nil, false
}

func toFloat(v value) float64 {
	switch n := toNumber(v).(type) {
	case int64:
		return float64(n)
	case float64:
		return n
	}
	return 0
}

// toInt converts v to an int64, truncating a float.
func toInt(v value) int64 {
	switch n := toNumber(v).(type) {
	case int64:
		return n
	case float64:
		if math.IsNaN(n) {
			return 0
		}
		return int64(max(min(n, math.MaxInt64), math.MinInt64))
	}
	return 0
}

func isNumber(v value) bool {
	switch v.(type) {
	case int64, float64:
		return true
	}
	return false
}

// isEmpty: "", an empty array or an empty range or map.
func isEmpty(v value) bool {
	switch v := v.(type) {
	case string:
		return v == ""
	case []value:
		return len(v) == 0
	case map[string]value:
		return len(v) == 0
	case rangeValue:
		return v.len() == 0
	}
	return false
}

// isBlank: nil, false, a string of white space only, or empty.
func isBlank(v value) bool {
	switch v := v.(type) {
	case nil:
		return true
	case bool:
		return !v
	case string:
		return strings.TrimSpace(v) == ""
	}
	return isEmpty(v)
}

// equals is ==: numbers by value (1 == 1.0), strings, booleans and nil exactly, arrays
// element by element; empty and blank test the other side. Values of different kinds
// are never equal ("1" != 1).
func equals(a, b value) bool {
	if s, ok := a.(special); ok {
		a, b = b, s
	}
	switch b := b.(type) {
	case special:
		if s, ok := a.(special); ok {
			return s == b
		}
		if b == emptyValue {
			return isEmpty(a)
		}
		return isBlank(a)
	case int64, float64:
		if !isNumber(a) {
			return false
		}
		if ai, ok := a.(int64); ok {
			if bi, ok := b.(int64); ok {
				return ai == bi
			}
		}
		return toFloat(a) == toFloat(b)
	case []value:
		as, ok := a.([]value)
		return ok && len(as) == len(b) && pairwiseEqual(as, b)
	case rangeValue:
		ar, ok := a.(rangeValue)
		return ok && ar == b
	case loopValue:
		return a == value(b)
	case map[string]value:
		am, ok := a.(map[string]value)
		if !ok || len(am) != len(b) {
			return false
		}
		for k, v := range b {
			if av, ok := am[k]; !ok || !equals(av, v) {
				return false
			}
		}
		return true
	}
	return a == b
}

func pairwiseEqual(a, b []value) bool {
	for i := range a {
		if !equals(a[i], b[i]) {
			return false
		}
	}
	return true
}

// order compares a and b for < and the like: numbers by value, strings byte by byte.
// ok is false for values that cannot be ordered against each other.
func order(a, b value) (cmp int, ok bool) {
	if isNumber(a) && isNumber(b) {
		if ai, aok := a.(int64); aok {
			if bi, bok := b.(int64); bok {
				return compareInts(ai, bi), true
			}
		}
		fa, fb := toFloat(a), toFloat(b)
		if math.IsNaN(fa) || math.IsNaN(fb) {
			return 0, false
		}
		switch {
		case fa < fb:
			return -1, true
		case fa > fb:
			return 1, true
		}
		return 0, true
	}
	as, aok := a.(string)
	bs, bok := b.(string)
	if aok && bok {
		return strings.Compare(as, bs), true
	}
	return 0, false
}

func compareInts(a, b int64) int {
	switch {
	case a < b:
		return -1
	case a > b:
		return 1
	}
	return 0
}

// contains: a substring of a string, an element of an array or range, or a key of a map.
func contains(a, b value) bool {
	switch a := a.(type) {
	case string:
		s, ok := b.(string)
		if !ok {
			if b == nil {
				return false
			}
			s = toString(b)
		}
		return strings.Contains(a, s)
	case []value:
		return slices.ContainsFunc(a, func(e value) bool { return equals(e, b) })
	case rangeValue:
		n, ok := b.(int64)
		return ok && n >= a.lo && n <= a.hi
	case map[string]value:
		s, ok := b.(string)
		_, has := a[s]
		return ok && has
	}
	return false
}

// property looks up v.key or v[key]: a map's key, an array's index (negative from the
// end), and size, first and last of arrays, ranges and strings (size only).
func property(v, key value) value {
	switch v := v.(type) {
	case loopValue:
		if k, ok := key.(string); ok {
			return v.get(k)
		}
	case map[string]value:
		if k, ok := key.(string); ok {
			return v[k]
		}
	case []value:
		switch k := key.(type) {
		case int64:
			if k < 0 {
				k += int64(len(v))
			}
			if k >= 0 && k < int64(len(v)) {
				return v[k]
			}
		case string:
			switch k {
			case "size":
				return int64(len(v))
			case "first":
				if len(v) > 0 {
					return v[0]
				}
			case "last":
				if len(v) > 0 {
					return v[len(v)-1]
				}
			}
		}
	case rangeValue:
		n := v.len()
		switch k := key.(type) {
		case int64:
			if k < 0 {
				k += n
			}
			if k >= 0 && k < n {
				return v.lo + k
			}
		case string:
			switch k {
			case "size":
				return n
			case "first":
				if n > 0 {
					return v.lo
				}
			case "last":
				if n > 0 {
					return v.hi
				}
			}
		}
	case string:
		if key == "size" {
			return int64(utf8.RuneCountInString(v))
		}
	}
	return nil
}
