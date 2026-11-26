// Package condition is the client condition model (spec §6.1, §7.3): a tree of nodes, each
// with exactly one predicate and an optional "not". It parses and validates conditions and
// decides whether one is empty. Evaluation against a request arrives with the proxy (I10).
//
// The package is backend code: standard library only.
package condition

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/netip"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// Condition is one node of a client condition. Exactly one of And, Or, HeaderMatch,
// HeaderRegex, IP and CIDR is set; Not negates the node. And and Or distinguish an empty
// list (set, [] in JSON) from an absent one (nil).
//
// Decoding uses the tags below; a strict decoder (DisallowUnknownFields) stays strict
// inside the tree. Encoding goes through MarshalJSON.
type Condition struct {
	And         []Condition `json:"and"`
	Or          []Condition `json:"or"`
	Not         bool        `json:"not"`
	HeaderMatch *Header     `json:"headerMatch"`
	HeaderRegex *Header     `json:"headerRegex"`
	IP          *string     `json:"ip"`
	CIDR        *string     `json:"cidr"`
}

// Header is the argument of headerMatch and headerRegex.
type Header struct {
	Header string `json:"header"`
	Value  string `json:"value"`
}

// wire is a Condition as JSON (spec §6.1): absent predicates are omitted, an empty and/or
// list is kept as [].
type wire struct {
	And         *[]Condition `json:"and,omitempty"`
	Or          *[]Condition `json:"or,omitempty"`
	Not         bool         `json:"not,omitempty"`
	HeaderMatch *Header      `json:"headerMatch,omitempty"`
	HeaderRegex *Header      `json:"headerRegex,omitempty"`
	IP          *string      `json:"ip,omitempty"`
	CIDR        *string      `json:"cidr,omitempty"`
}

// MarshalJSON writes the node in the shape of spec §6.1.
func (c Condition) MarshalJSON() ([]byte, error) {
	w := wire{Not: c.Not, HeaderMatch: c.HeaderMatch, HeaderRegex: c.HeaderRegex, IP: c.IP, CIDR: c.CIDR}
	if c.And != nil {
		w.And = &c.And
	}
	if c.Or != nil {
		w.Or = &c.Or
	}
	return json.Marshal(w)
}

// ErrSyntax wraps the reasons Parse cannot read a condition.
var ErrSyntax = errors.New("not a valid condition")

// Parse reads a condition written as JSON, as the UI's text field has it. Empty text and
// null are no condition (nil). Unknown fields and trailing data are errors.
func Parse(text string) (*Condition, error) {
	text = strings.TrimSpace(text)
	if text == "" || text == "null" {
		return nil, nil
	}
	dec := json.NewDecoder(strings.NewReader(text))
	dec.DisallowUnknownFields()
	var c Condition
	if err := dec.Decode(&c); err != nil {
		return nil, fmt.Errorf("%w: %s", ErrSyntax, jsonProblem(err))
	}
	if _, err := dec.Token(); !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("%w: unexpected text after the condition", ErrSyntax)
	}
	return &c, nil
}

// jsonProblem turns an encoding/json error into a short message for the UI.
func jsonProblem(err error) string {
	var syntax *json.SyntaxError
	var typ *json.UnmarshalTypeError
	switch {
	case errors.As(err, &syntax):
		return fmt.Sprintf("invalid JSON at character %d", syntax.Offset)
	case errors.Is(err, io.ErrUnexpectedEOF), errors.Is(err, io.EOF):
		return "the JSON ends too early"
	case errors.As(err, &typ):
		if typ.Field != "" {
			return fmt.Sprintf("%s has the wrong type", typ.Field)
		}
		return "a value has the wrong type"
	}
	return strings.TrimPrefix(err.Error(), "json: ")
}

// Problem is one validation failure. Path is relative to the condition's root, in the
// notation of spec §6.2 ("or[0].cidr"); "" is the root node itself.
type Problem struct {
	Path    string
	Message string
}

// Limits from spec §6.4.
const maxHeaderText = 255

// Validate checks c at every level (spec §6.4): exactly one predicate per node, header
// names and values of 1–255 characters, a valid IP address, a valid CIDR prefix and a
// headerRegex that compiles as RE2. A nil condition is valid.
func Validate(c *Condition) []Problem {
	if c == nil {
		return nil
	}
	var out []Problem
	validate(*c, "", &out)
	return out
}

func validate(c Condition, path string, out *[]Problem) {
	add := func(field, msg string) { *out = append(*out, Problem{Path: join(path, field), Message: msg}) }
	n := 0
	for _, set := range []bool{c.And != nil, c.Or != nil, c.HeaderMatch != nil, c.HeaderRegex != nil, c.IP != nil, c.CIDR != nil} {
		if set {
			n++
		}
	}
	if n != 1 {
		add("", fmt.Sprintf("must have exactly one of and, or, headerMatch, headerRegex, ip, cidr (has %d)", n))
		return
	}
	switch {
	case c.And != nil:
		for i, ch := range c.And {
			validate(ch, join(path, "and")+"["+strconv.Itoa(i)+"]", out)
		}
	case c.Or != nil:
		for i, ch := range c.Or {
			validate(ch, join(path, "or")+"["+strconv.Itoa(i)+"]", out)
		}
	case c.HeaderMatch != nil:
		validateHeader(*c.HeaderMatch, join(path, "headerMatch"), out)
	case c.HeaderRegex != nil:
		p := join(path, "headerRegex")
		validateHeader(*c.HeaderRegex, p, out)
		if _, err := regexp.Compile(c.HeaderRegex.Value); err != nil {
			*out = append(*out, Problem{Path: join(p, "value"), Message: "is not a valid regular expression: " + strings.TrimPrefix(err.Error(), "error parsing regexp: ")})
		}
	case c.IP != nil:
		if a, err := netip.ParseAddr(*c.IP); err != nil || a.Zone() != "" {
			add("ip", "must be an IPv4 or IPv6 address")
		}
	case c.CIDR != nil:
		if p, err := netip.ParsePrefix(*c.CIDR); err != nil || p.Addr().Zone() != "" {
			add("cidr", "must be a CIDR prefix, such as 10.0.0.0/8 or 2001:db8::/32")
		}
	}
}

func validateHeader(h Header, path string, out *[]Problem) {
	for _, f := range []struct{ name, value string }{{"header", h.Header}, {"value", h.Value}} {
		if n := utf8.RuneCountInString(f.value); n < 1 || n > maxHeaderText {
			*out = append(*out, Problem{Path: join(path, f.name), Message: fmt.Sprintf("must be 1–%d characters", maxHeaderText)})
		}
	}
}

func join(path, field string) string {
	switch {
	case field == "":
		return path
	case path == "":
		return field
	}
	return path + "." + field
}

// Empty reports whether c is empty (spec §7.3): nil, or without any ip, cidr,
// headerMatch or headerRegex predicate anywhere in its tree. {"and": []} is empty. An
// empty condition cannot be enabled.
func Empty(c *Condition) bool {
	return c == nil || !hasLeaf(*c)
}

func hasLeaf(c Condition) bool {
	if c.HeaderMatch != nil || c.HeaderRegex != nil || c.IP != nil || c.CIDR != nil {
		return true
	}
	for _, ch := range c.And {
		if hasLeaf(ch) {
			return true
		}
	}
	for _, ch := range c.Or {
		if hasLeaf(ch) {
			return true
		}
	}
	return false
}
