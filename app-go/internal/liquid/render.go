package liquid

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

type renderer struct {
	src    string
	now    time.Time
	loc    *time.Location
	scopes []map[string]value // [0] is the global scope that assign and capture write to
	steps  int
	size   int
}

// control is how a list of nodes ended: normally, or at a break or continue.
type control int

const (
	ctrlNone control = iota
	ctrlBreak
	ctrlContinue
)

func (r *renderer) errAt(pos int, msg string) error { return errorAt(r.src, pos, msg) }

// step counts n units of work against MaxSteps.
func (r *renderer) step(n int, pos int) error {
	if r.steps += n; r.steps > MaxSteps || n < 0 {
		return r.errAt(pos, fmt.Sprintf("the template takes more than %d steps", MaxSteps))
	}
	return nil
}

// grow counts n bytes produced against MaxSize.
func (r *renderer) grow(n int, pos int) error {
	if r.size += n; r.size > MaxSize || n < 0 {
		return r.errAt(pos, fmt.Sprintf("the template produces more than %d MB", MaxSize>>20))
	}
	return nil
}

// reserve checks, before a filter makes a string of n bytes, that it would fit in
// MaxSize; the string is counted once made.
func (r *renderer) reserve(n int, pos int) error {
	if r.size+n > MaxSize || n < 0 {
		return r.errAt(pos, fmt.Sprintf("the template produces more than %d MB", MaxSize>>20))
	}
	return nil
}

func (r *renderer) write(out *strings.Builder, s string, pos int) error {
	if err := r.grow(len(s), pos); err != nil {
		return err
	}
	out.WriteString(s)
	return nil
}

func (r *renderer) render(nodes []node, out *strings.Builder) (control, error) {
	for _, n := range nodes {
		pos := n.position()
		if err := r.step(1, pos); err != nil {
			return ctrlNone, err
		}
		switch n := n.(type) {
		case textNode:
			if err := r.write(out, n.text, pos); err != nil {
				return ctrlNone, err
			}
		case outputNode:
			v, err := r.filtered(n.value)
			if err != nil {
				return ctrlNone, err
			}
			if err := r.write(out, toString(v), pos); err != nil {
				return ctrlNone, err
			}
		case assignNode:
			v, err := r.filtered(n.value)
			if err != nil {
				return ctrlNone, err
			}
			r.scopes[0][n.name] = v
		case captureNode:
			var b strings.Builder
			c, err := r.render(n.body, &b)
			if err != nil {
				return ctrlNone, err
			}
			r.scopes[0][n.name] = b.String()
			if c != ctrlNone {
				return c, nil
			}
		case ifNode:
			c, err := r.ifNode(n, out)
			if err != nil || c != ctrlNone {
				return c, err
			}
		case caseNode:
			c, err := r.caseNode(n, out)
			if err != nil || c != ctrlNone {
				return c, err
			}
		case forNode:
			if err := r.forNode(n, out); err != nil {
				return ctrlNone, err
			}
		case breakNode:
			return ctrlBreak, nil
		case continueNode:
			return ctrlContinue, nil
		}
	}
	return ctrlNone, nil
}

func (r *renderer) ifNode(n ifNode, out *strings.Builder) (control, error) {
	for k, b := range n.branches {
		v, err := r.eval(b.cond)
		if err != nil {
			return ctrlNone, err
		}
		holds := truthy(v)
		if k == 0 && n.unless {
			holds = !holds
		}
		if holds {
			return r.render(b.body, out)
		}
	}
	return r.render(n.elseBody, out)
}

func (r *renderer) caseNode(n caseNode, out *strings.Builder) (control, error) {
	subject, err := r.eval(n.subject)
	if err != nil {
		return ctrlNone, err
	}
	for _, w := range n.whens {
		for _, e := range w.values {
			v, err := r.eval(e)
			if err != nil {
				return ctrlNone, err
			}
			if equals(subject, v) {
				return r.render(w.body, out)
			}
		}
	}
	return r.render(n.elseBody, out)
}

// forNode renders a for loop. offset and limit are applied first, then reversed. A
// range is walked without being made into an array.
func (r *renderer) forNode(n forNode, out *strings.Builder) error {
	coll, err := r.eval(n.collection)
	if err != nil {
		return err
	}
	var items []value
	rng, isRange := coll.(rangeValue)
	if !isRange {
		if items, err = r.enumerable(coll, n.pos); err != nil {
			return err
		}
		rng = rangeValue{0, int64(len(items)) - 1}
	}
	lo, hi := rng.lo, rng.hi
	if n.offset != nil {
		v, err := r.eval(n.offset)
		if err != nil {
			return err
		}
		if off := toInt(v); off > 0 {
			lo = satAdd(lo, off)
		}
	}
	if n.limit != nil {
		v, err := r.eval(n.limit)
		if err != nil {
			return err
		}
		hi = min(hi, satAdd(lo, max(toInt(v), 0)-1))
	}
	length := rangeValue{lo, hi}.len()
	if length == 0 {
		_, err := r.render(n.elseBody, out)
		return err
	}
	scope := map[string]value{}
	r.scopes = append(r.scopes, scope)
	defer func() { r.scopes = r.scopes[:len(r.scopes)-1] }()
	for k := int64(0); k < length; k++ {
		if err := r.step(1, n.pos); err != nil {
			return err
		}
		i := lo + k
		if n.reversed {
			i = hi - k
		}
		if isRange {
			scope[n.name] = i
		} else {
			scope[n.name] = items[i]
		}
		scope["forloop"] = loopValue{k, length}
		c, err := r.render(n.body, out)
		if err != nil {
			return err
		}
		if c == ctrlBreak {
			break
		}
	}
	return nil
}

// satAdd adds without overflowing.
func satAdd(a, b int64) int64 {
	s := a + b
	if b > 0 && s < a {
		return 1<<63 - 1
	}
	return s
}

// enumerable is what a for loop walks for a value that is not a range: an array's
// elements, a map's [key, value] pairs in key order, a non-empty string as one item,
// and nothing for anything else.
func (r *renderer) enumerable(v value, pos int) ([]value, error) {
	switch v := v.(type) {
	case []value:
		return v, nil
	case map[string]value:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		out := make([]value, len(keys))
		for i, k := range keys {
			out[i] = []value{k, v[k]}
		}
		return out, r.step(len(out), pos)
	case string:
		if v != "" {
			return []value{v}, nil
		}
	}
	return nil, nil
}

// array is v as an array for the array filters: a range made into one, nil as none,
// and any other single value as an array of it.
func (r *renderer) array(v value, pos int) ([]value, error) {
	switch v := v.(type) {
	case []value:
		return v, nil
	case rangeValue:
		n := v.len()
		if n > MaxSteps {
			return nil, r.step(MaxSteps+1, pos)
		}
		if err := r.step(int(n), pos); err != nil {
			return nil, err
		}
		out := make([]value, n)
		for i := range out {
			out[i] = v.lo + int64(i)
		}
		return out, nil
	case nil:
		return nil, nil
	}
	return []value{v}, nil
}

func (r *renderer) filtered(f *filtered) (value, error) {
	v, err := r.eval(f.base)
	if err != nil {
		return nil, err
	}
	for _, call := range f.filters {
		args := make([]value, len(call.args))
		for i, a := range call.args {
			if args[i], err = r.eval(a); err != nil {
				return nil, err
			}
		}
		var named map[string]value
		for k, a := range call.named {
			if named == nil {
				named = map[string]value{}
			}
			if named[k], err = r.eval(a); err != nil {
				return nil, err
			}
		}
		fn, ok := filters[call.name]
		if !ok {
			v = nil // an unknown filter renders as an empty string (spec §8.2)
			continue
		}
		if v, err = fn(r, filterInput{v: v, args: args, named: named, pos: call.pos}); err != nil {
			return nil, err
		}
		if s, ok := v.(string); ok {
			if err := r.grow(len(s), call.pos); err != nil {
				return nil, err
			}
		}
	}
	return v, nil
}

func (r *renderer) eval(e expr) (value, error) {
	switch e := e.(type) {
	case literal:
		return e.v, nil
	case variable:
		return r.variable(e)
	case rangeExpr:
		lo, err := r.eval(e.lo)
		if err != nil {
			return nil, err
		}
		hi, err := r.eval(e.hi)
		if err != nil {
			return nil, err
		}
		return rangeValue{toInt(lo), toInt(hi)}, nil
	case compare:
		l, err := r.eval(e.l)
		if err != nil {
			return nil, err
		}
		rv, err := r.eval(e.r)
		if err != nil {
			return nil, err
		}
		return compareValues(e.op, l, rv), nil
	case logic:
		l, err := r.eval(e.l)
		if err != nil {
			return nil, err
		}
		if e.and && !truthy(l) {
			return false, nil
		}
		if !e.and && truthy(l) {
			return true, nil
		}
		rv, err := r.eval(e.r)
		if err != nil {
			return nil, err
		}
		return truthy(rv), nil
	case *filtered:
		return r.filtered(e)
	}
	return nil, nil
}

func compareValues(op string, l, r value) bool {
	switch op {
	case "==":
		return equals(l, r)
	case "!=", "<>":
		return !equals(l, r)
	case "contains":
		return contains(l, r)
	}
	c, ok := order(l, r)
	if !ok {
		return false
	}
	switch op {
	case "<":
		return c < 0
	case ">":
		return c > 0
	case "<=":
		return c <= 0
	case ">=":
		return c >= 0
	}
	return false
}

// variable looks the name up from the innermost scope out (undefined is nil), then
// follows the lookups.
func (r *renderer) variable(e variable) (value, error) {
	var v value
	for i := len(r.scopes) - 1; i >= 0; i-- {
		if x, ok := r.scopes[i][e.name]; ok {
			v = x
			break
		}
	}
	for _, a := range e.path {
		var key value = a.key
		if a.index != nil {
			var err error
			if key, err = r.eval(a.index); err != nil {
				return nil, err
			}
			if f, ok := key.(float64); ok && f == float64(int64(f)) {
				key = int64(f)
			}
		}
		v = property(v, key)
	}
	return v, nil
}
