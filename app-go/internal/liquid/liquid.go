// Package liquid is the Liquid template subset that response bodies are rendered with
// (spec §8): output with filters, assign, capture, if/unless, case, for, raw and
// comment, over an empty context.
//
// Parse checks the syntax and reports the line and column of an error; Render runs a
// parsed template. Rendering is bounded (MaxSteps, MaxSize), so a template cannot keep a
// proxy request busy for long or use much memory, and the few errors that remain
// possible at run time (a division by zero, a budget exceeded) are returned, never
// panicked.
//
// The package is backend code: standard library only.
package liquid

import (
	"fmt"
	"strings"
	"time"

	// The date filter formats in the process's local zone (TZ, spec §8.2), also in an
	// image without a zoneinfo database.
	_ "time/tzdata"
)

// Marker starts a response body that is a template (spec §8.1).
const Marker = "<% TEMPLATE: LiquidJS %>"

// Limits.
const (
	// MaxDepth is how deeply blocks (if, for, …) and [ ] lookups may nest.
	MaxDepth = 64
	// MaxSteps bounds the work of one render: tags and outputs run, loop iterations
	// and array elements made.
	MaxSteps = 1_000_000
	// MaxSize bounds the bytes of all strings one render produces: the output, and
	// what captures and filters make along the way.
	MaxSize = 16 << 20
)

// IsTemplate reports whether a response body is a template: it starts with Marker.
func IsTemplate(body string) bool { return strings.HasPrefix(body, Marker) }

// Error is a syntax or render error. Line and Column are 1-based and count characters,
// not bytes, in the text that was parsed (for ParseBody, the body with its marker).
type Error struct {
	Offset int // byte offset in the source
	Line   int
	Column int
	Msg    string
}

func (e *Error) Error() string {
	return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Msg)
}

// errorAt returns the error msg at byte offset pos of src, with its line and column.
func errorAt(src string, pos int, msg string) *Error {
	pos = max(0, min(pos, len(src)))
	line, col := 1, 1
	for _, r := range src[:pos] {
		if r == '\n' {
			line, col = line+1, 1
		} else {
			col++
		}
	}
	return &Error{Offset: pos, Line: line, Column: col, Msg: msg}
}

// Template is a parsed template. It is immutable and safe for concurrent use.
type Template struct {
	src   string
	nodes []node
}

// Parse parses template source.
func Parse(src string) (*Template, error) { return parse(src, 0) }

// ParseBody parses a response body that is a template (IsTemplate): what follows the
// marker. Error positions are in body, marker included. It returns nil, nil for a body
// that is not a template.
func ParseBody(body string) (*Template, error) {
	if !IsTemplate(body) {
		return nil, nil
	}
	return parse(body, len(Marker))
}

// Options are the inputs of a render besides the template.
type Options struct {
	Now      func() time.Time // nil means time.Now; called once per render
	Location *time.Location   // the zone dates are shown in; nil means time.Local
}

// Render renders the template with an empty context: only literals, assign and
// capture provide values (spec §8.1). 'now' is read once, so every use in one render
// sees the same time.
func (t *Template) Render(opts Options) (string, error) {
	now := time.Now
	if opts.Now != nil {
		now = opts.Now
	}
	loc := opts.Location
	if loc == nil {
		loc = time.Local
	}
	r := &renderer{src: t.src, now: now().In(loc), loc: loc, scopes: []map[string]value{{}}}
	var out strings.Builder
	if _, err := r.render(t.nodes, &out); err != nil {
		return "", err
	}
	return out.String(), nil
}
