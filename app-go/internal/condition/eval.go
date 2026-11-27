package condition

import (
	"fmt"
	"net/http"
	"net/netip"
	"regexp"
	"strings"
)

// Request is what a client condition is evaluated against (spec §7.3).
type Request struct {
	// IP is the client IP (internal/clientip), IPv4-mapped addresses already unmapped. The
	// zero Addr (no usable address) matches no ip or cidr predicate.
	IP     netip.Addr
	Header http.Header
}

// Matcher is a compiled condition: regular expressions, addresses and prefixes are parsed
// once, when the catalog is built, not on every request. A Matcher is immutable and safe
// for concurrent use.
type Matcher struct{ root node }

type kind int

const (
	kindAnd kind = iota
	kindOr
	kindIP
	kindCIDR
	kindHeaderMatch
	kindHeaderRegex
)

type node struct {
	kind     kind
	not      bool
	children []node
	ip       netip.Addr
	prefix   netip.Prefix
	header   string // canonical header name
	value    string
	re       *regexp.Regexp
}

// Compile turns a valid condition (see Validate) into a Matcher. It returns an error for a
// condition Validate would reject.
func Compile(c *Condition) (*Matcher, error) {
	if c == nil {
		return nil, fmt.Errorf("%w: no condition", ErrSyntax)
	}
	if problems := Validate(c); len(problems) > 0 {
		p := problems[0]
		return nil, fmt.Errorf("%w: %s %s", ErrSyntax, p.Path, p.Message)
	}
	return &Matcher{root: compile(*c)}, nil
}

// compile converts a validated node.
func compile(c Condition) node {
	n := node{not: c.Not}
	switch {
	case c.And != nil:
		n.kind = kindAnd
		n.children = compileAll(c.And)
	case c.Or != nil:
		n.kind = kindOr
		n.children = compileAll(c.Or)
	case c.IP != nil:
		n.kind = kindIP
		n.ip = netip.MustParseAddr(*c.IP).Unmap()
	case c.CIDR != nil:
		n.kind = kindCIDR
		n.prefix = unmapPrefix(netip.MustParsePrefix(*c.CIDR))
	case c.HeaderMatch != nil:
		n.kind = kindHeaderMatch
		n.header = http.CanonicalHeaderKey(c.HeaderMatch.Header)
		n.value = c.HeaderMatch.Value
	case c.HeaderRegex != nil:
		n.kind = kindHeaderRegex
		n.header = http.CanonicalHeaderKey(c.HeaderRegex.Header)
		n.re = regexp.MustCompile(c.HeaderRegex.Value)
	}
	return n
}

func compileAll(cs []Condition) []node {
	out := make([]node, len(cs))
	for i, c := range cs {
		out[i] = compile(c)
	}
	return out
}

// unmapPrefix turns an IPv4-mapped IPv6 prefix (::ffff:10.0.0.0/104) into the IPv4 prefix
// it covers (10.0.0.0/8), so it matches the unmapped client IP (spec §7.3).
func unmapPrefix(p netip.Prefix) netip.Prefix {
	p = p.Masked()
	if p.Addr().Is4In6() && p.Bits() >= 96 {
		return netip.PrefixFrom(p.Addr().Unmap(), p.Bits()-96)
	}
	return p
}

// Match reports whether the condition is true for r (spec §7.3).
func (m *Matcher) Match(r Request) bool { return m.root.match(r) }

func (n *node) match(r Request) bool {
	var v bool
	switch n.kind {
	case kindAnd:
		v = true
		for i := range n.children {
			if !n.children[i].match(r) {
				v = false
				break
			}
		}
	case kindOr:
		for i := range n.children {
			if n.children[i].match(r) {
				v = true
				break
			}
		}
	case kindIP:
		v = r.IP.IsValid() && r.IP == n.ip
	case kindCIDR:
		v = r.IP.IsValid() && n.prefix.Contains(r.IP)
	case kindHeaderMatch:
		value, ok := headerValue(r.Header, n.header)
		v = ok && value == n.value // a missing header never matches
	case kindHeaderRegex:
		value, _ := headerValue(r.Header, n.header) // a missing header is ""
		v = n.re.MatchString(value)
	}
	return v != n.not
}

// headerValue returns every value of the header name, joined with ", " (spec §7.3), and
// whether the header is present. Names are compared case-insensitively: the server stores
// incoming names canonicalised, and a name it could not canonicalise is found by a scan.
func headerValue(h http.Header, name string) (string, bool) {
	values, ok := h[name]
	if !ok {
		for k, v := range h {
			if strings.EqualFold(k, name) {
				values, ok = v, true
				break
			}
		}
	}
	if !ok {
		return "", false
	}
	return strings.Join(values, ", "), true
}
