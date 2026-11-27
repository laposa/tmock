// Package expr is the request-condition language of scenarios (spec §7.4): a small, safe
// replacement for the legacy app's JavaScript eval. An expression compares values taken
// from $request with literals, and combines comparisons with &&, || and !.
//
//	expr     := or
//	or       := and ( "||" and )*
//	and      := unary ( "&&" unary )*
//	unary    := "!" unary | cmp
//	cmp      := primary ( op primary )?          op ∈ == != < <= > >=   (=== and !== are aliases)
//	primary  := literal | ref | "(" expr ")"
//	ref      := "$request" ( "." ident | "[" ( string | integer ) "]" )*
//	literal  := 'single' | "double" | number | true | false | null
//
// Parse checks the syntax once (when a scenario is saved, and when the catalog is built);
// an Expr is immutable and safe for concurrent use, and evaluating it cannot fail.
//
// The package is backend code: standard library only.
package expr

import (
	"fmt"
	"math/big"
	"strconv"
	"strings"
	"unicode/utf8"
)

// MaxDepth is how deeply parentheses and ! may nest.
const MaxDepth = 64

// Error is a syntax error. Line and Column are 1-based and count characters, not bytes.
type Error struct {
	Offset int // byte offset in the source
	Line   int
	Column int
	Msg    string
}

func (e *Error) Error() string {
	if e.Line > 1 {
		return fmt.Sprintf("line %d, column %d: %s", e.Line, e.Column, e.Msg)
	}
	return fmt.Sprintf("column %d: %s", e.Column, e.Msg)
}

// Expr is a parsed expression.
type Expr struct {
	root     node
	usesBody bool
}

// UsesBody reports whether evaluating the expression needs the request body: it refers to
// $request.body, or to $request as a whole.
func (e *Expr) UsesBody() bool { return e.usesBody }

type nodeKind int

const (
	nodeLiteral nodeKind = iota
	nodeRef
	nodeNot
	nodeAnd
	nodeOr
	nodeCmp
)

type node struct {
	kind  nodeKind
	value any    // nodeLiteral: nil, bool, *big.Float (never modified) or string
	path  []any  // nodeRef: the segments after $request: string (key) or int (index)
	op    string // nodeCmp: == != < <= > >=
	kids  []node // nodeNot: 1; nodeAnd, nodeOr: 2 or more; nodeCmp: 2
}

// Parse parses src. A syntax error is an *Error.
func Parse(src string) (*Expr, error) {
	p := &parser{src: src}
	if err := p.lex(); err != nil {
		return nil, err
	}
	root, err := p.or(0)
	if err != nil {
		return nil, err
	}
	if t := p.peek(); t.kind != tokEOF {
		return nil, p.errAt(t.pos, "unexpected %s", t.describe())
	}
	return &Expr{root: root, usesBody: p.usesBody}, nil
}

type tokKind int

const (
	tokEOF tokKind = iota
	tokString
	tokNumber
	tokIdent // true, false, null, and names after "."
	tokRequest
	tokOp // == != < <= > >= && || !
	tokDot
	tokLBracket
	tokRBracket
	tokLParen
	tokRParen
)

type token struct {
	kind tokKind
	pos  int
	text string     // tokIdent, tokOp: the text; tokString: the decoded value
	num  *big.Float // tokNumber
	raw  string     // tokNumber: as written
}

func (t token) describe() string {
	switch t.kind {
	case tokEOF:
		return "end of condition"
	case tokString:
		return "string " + strconv.Quote(t.text)
	case tokNumber:
		return "number " + t.raw
	case tokRequest:
		return "$request"
	case tokDot:
		return `"."`
	case tokLBracket:
		return `"["`
	case tokRBracket:
		return `"]"`
	case tokLParen:
		return `"("`
	case tokRParen:
		return `")"`
	}
	return strconv.Quote(t.text)
}

type parser struct {
	src      string
	toks     []token
	i        int
	usesBody bool
}

func (p *parser) errAt(pos int, format string, args ...any) *Error {
	return ErrorAt(p.src, pos, fmt.Sprintf(format, args...))
}

// ErrorAt returns the error msg at byte offset pos of src, with its line and column.
// Callers that parse a trimmed form of what the user wrote use it to report the
// position in the original text.
func ErrorAt(src string, pos int, msg string) *Error {
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

func (p *parser) peek() token { return p.toks[p.i] }

func (p *parser) next() token {
	t := p.toks[p.i]
	if t.kind != tokEOF {
		p.i++
	}
	return t
}

func isIdentStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }
func isDigit(c byte) bool      { return c >= '0' && c <= '9' }
func isIdentPart(c byte) bool  { return isIdentStart(c) || isDigit(c) }

// lex splits the source into tokens, ending with tokEOF.
func (p *parser) lex() error {
	s := p.src
	for i := 0; i < len(s); {
		c := s[i]
		switch {
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			i++
		case c == '\'' || c == '"':
			v, n, err := p.lexString(i)
			if err != nil {
				return err
			}
			p.toks = append(p.toks, token{kind: tokString, pos: i, text: v})
			i += n
		case isDigit(c) || c == '-' && i+1 < len(s) && isDigit(s[i+1]):
			j := i + 1
			for j < len(s) && (isDigit(s[j]) || s[j] == '.' || s[j] == 'e' || s[j] == 'E' ||
				(s[j] == '+' || s[j] == '-') && (s[j-1] == 'e' || s[j-1] == 'E')) {
				j++
			}
			if j < len(s) && isIdentStart(s[j]) {
				return p.errAt(j, "unexpected %q after a number", s[j])
			}
			raw := s[i:j]
			f, ok := parseNumber(raw)
			if !validNumber(raw) || !ok || f.IsInf() {
				return p.errAt(i, "%q is not a number", raw)
			}
			p.toks = append(p.toks, token{kind: tokNumber, pos: i, num: f, raw: raw})
			i = j
		case c == '$':
			j := i + 1
			for j < len(s) && isIdentPart(s[j]) {
				j++
			}
			if s[i:j] != "$request" {
				return p.errAt(i, "unknown name %q: only $request is available", s[i:j])
			}
			p.toks = append(p.toks, token{kind: tokRequest, pos: i, text: "$request"})
			i = j
		case isIdentStart(c):
			j := i + 1
			for j < len(s) && isIdentPart(s[j]) {
				j++
			}
			p.toks = append(p.toks, token{kind: tokIdent, pos: i, text: s[i:j]})
			i = j
		case c == '.':
			p.toks = append(p.toks, token{kind: tokDot, pos: i, text: "."})
			i++
		case c == '[':
			p.toks = append(p.toks, token{kind: tokLBracket, pos: i, text: "["})
			i++
		case c == ']':
			p.toks = append(p.toks, token{kind: tokRBracket, pos: i, text: "]"})
			i++
		case c == '(':
			p.toks = append(p.toks, token{kind: tokLParen, pos: i, text: "("})
			i++
		case c == ')':
			p.toks = append(p.toks, token{kind: tokRParen, pos: i, text: ")"})
			i++
		default:
			op := ""
			for _, cand := range []string{"===", "!==", "==", "!=", "<=", ">=", "&&", "||", "<", ">", "!"} {
				if strings.HasPrefix(s[i:], cand) {
					op = cand
					break
				}
			}
			if op == "" {
				if c == '=' {
					return p.errAt(i, `"=" is not an operator; use "==" to compare`)
				}
				r, _ := utf8.DecodeRuneInString(s[i:])
				return p.errAt(i, "unexpected character %q", r)
			}
			text := op
			switch op {
			case "===":
				text = "=="
			case "!==":
				text = "!="
			}
			p.toks = append(p.toks, token{kind: tokOp, pos: i, text: text})
			i += len(op)
		}
	}
	p.toks = append(p.toks, token{kind: tokEOF, pos: len(s)})
	return nil
}

// lexString reads the quoted string at s[start] and returns its value and length.
func (p *parser) lexString(start int) (string, int, error) {
	s := p.src
	quote := s[start]
	var b strings.Builder
	for i := start + 1; i < len(s); {
		c := s[i]
		switch {
		case c == quote:
			return b.String(), i + 1 - start, nil
		case c == '\n':
			return "", 0, p.errAt(i, "line break in a string")
		case c == '\\':
			if i+1 >= len(s) {
				return "", 0, p.errAt(start, "unterminated string")
			}
			switch e := s[i+1]; e {
			case '\\', '\'', '"', '/':
				b.WriteByte(e)
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			case 'u':
				if i+6 > len(s) {
					return "", 0, p.errAt(i, `"\u" needs four hex digits`)
				}
				n, err := strconv.ParseUint(s[i+2:i+6], 16, 16) // four hex digits: at most 0xFFFF
				if err != nil {
					return "", 0, p.errAt(i, `"\u" needs four hex digits`)
				}
				b.WriteRune(rune(n))
				i += 6
				continue
			default:
				return "", 0, p.errAt(i, "unknown escape %q", s[i:i+2])
			}
			i += 2
		default:
			b.WriteByte(c)
			i++
		}
	}
	return "", 0, p.errAt(start, "unterminated string")
}

func (p *parser) or(depth int) (node, error) {
	return p.binary(depth, "||", nodeOr, p.and)
}

func (p *parser) and(depth int) (node, error) {
	return p.binary(depth, "&&", nodeAnd, p.unary)
}

func (p *parser) binary(depth int, op string, kind nodeKind, operand func(int) (node, error)) (node, error) {
	first, err := operand(depth)
	if err != nil {
		return node{}, err
	}
	kids := []node{first}
	for t := p.peek(); t.kind == tokOp && t.text == op; t = p.peek() {
		p.next()
		n, err := operand(depth)
		if err != nil {
			return node{}, err
		}
		kids = append(kids, n)
	}
	if len(kids) == 1 {
		return first, nil
	}
	return node{kind: kind, kids: kids}, nil
}

func (p *parser) unary(depth int) (node, error) {
	if t := p.peek(); t.kind == tokOp && t.text == "!" {
		if depth >= MaxDepth {
			return node{}, p.errAt(t.pos, "the condition is nested too deeply")
		}
		p.next()
		n, err := p.unary(depth + 1)
		if err != nil {
			return node{}, err
		}
		return node{kind: nodeNot, kids: []node{n}}, nil
	}
	return p.cmp(depth)
}

func isCmp(op string) bool {
	switch op {
	case "==", "!=", "<", "<=", ">", ">=":
		return true
	}
	return false
}

func (p *parser) cmp(depth int) (node, error) {
	left, err := p.primary(depth)
	if err != nil {
		return node{}, err
	}
	t := p.peek()
	if t.kind != tokOp || !isCmp(t.text) {
		return left, nil
	}
	p.next()
	right, err := p.primary(depth)
	if err != nil {
		return node{}, err
	}
	if t2 := p.peek(); t2.kind == tokOp && isCmp(t2.text) {
		return node{}, p.errAt(t2.pos, "comparisons cannot be chained; use && or parentheses")
	}
	return node{kind: nodeCmp, op: t.text, kids: []node{left, right}}, nil
}

func (p *parser) primary(depth int) (node, error) {
	t := p.next()
	switch t.kind {
	case tokString:
		return node{kind: nodeLiteral, value: t.text}, nil
	case tokNumber:
		return node{kind: nodeLiteral, value: t.num}, nil
	case tokIdent:
		switch t.text {
		case "true":
			return node{kind: nodeLiteral, value: true}, nil
		case "false":
			return node{kind: nodeLiteral, value: false}, nil
		case "null":
			return node{kind: nodeLiteral, value: nil}, nil
		}
		return node{}, p.errAt(t.pos, "unknown name %q: values come from $request, strings need quotes", t.text)
	case tokRequest:
		return p.ref()
	case tokLParen:
		if depth >= MaxDepth {
			return node{}, p.errAt(t.pos, "the condition is nested too deeply")
		}
		n, err := p.or(depth + 1)
		if err != nil {
			return node{}, err
		}
		if c := p.next(); c.kind != tokRParen {
			return node{}, p.errAt(c.pos, `expected ")" but found %s`, c.describe())
		}
		return n, nil
	}
	return node{}, p.errAt(t.pos, "expected a value but found %s", t.describe())
}

// ref parses the segments after $request.
func (p *parser) ref() (node, error) {
	var path []any
	for {
		switch t := p.peek(); t.kind {
		case tokDot:
			p.next()
			id := p.next()
			if id.kind != tokIdent {
				return node{}, p.errAt(id.pos, `expected a name after "." but found %s`, id.describe())
			}
			path = append(path, id.text)
		case tokLBracket:
			p.next()
			k := p.next()
			switch {
			case k.kind == tokString:
				path = append(path, k.text)
			case k.kind == tokNumber && !strings.ContainsAny(k.raw, ".eE-"):
				i, err := strconv.Atoi(k.raw)
				if err != nil {
					return node{}, p.errAt(k.pos, "index %s is too large", k.raw)
				}
				path = append(path, i)
			default:
				return node{}, p.errAt(k.pos, `expected a string or a whole number in "[]" but found %s`, k.describe())
			}
			if c := p.next(); c.kind != tokRBracket {
				return node{}, p.errAt(c.pos, `expected "]" but found %s`, c.describe())
			}
		default:
			if len(path) == 0 || path[0] == "body" {
				p.usesBody = true
			}
			return node{kind: nodeRef, path: path}, nil
		}
	}
}
