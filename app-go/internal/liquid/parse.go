package liquid

import (
	"fmt"
	"slices"
)

// ── Syntax tree ─────────────────────────────────────────────────────────────────

// at is where a node starts in the source, for render errors.
type at struct{ pos int }

func (a at) position() int { return a.pos }

type node interface{ position() int }

type (
	textNode struct {
		at
		text string
	}
	outputNode struct {
		at
		value *filtered
	}
	assignNode struct {
		at
		name  string
		value *filtered
	}
	captureNode struct {
		at
		name string
		body []node
	}
	// ifNode is if and unless: the first branch whose condition holds (does not hold,
	// for the first branch of unless) is rendered, otherwise the else body.
	ifNode struct {
		at
		unless   bool
		branches []branch
		elseBody []node
	}
	caseNode struct {
		at
		subject  expr
		whens    []when
		elseBody []node
	}
	forNode struct {
		at
		name          string
		collection    expr
		limit, offset expr // nil when not given
		reversed      bool
		body          []node
		elseBody      []node
	}
	breakNode    struct{ at }
	continueNode struct{ at }
)

type branch struct {
	cond expr
	body []node
}

type when struct {
	values []expr
	body   []node
}

type expr interface{ isExpr() }

type (
	literal struct{ v value }
	// variable is name followed by .key and [expr] lookups.
	variable struct {
		name string
		path []accessor
	}
	rangeExpr struct{ lo, hi expr }
	// compare is l op r; op is ==, !=, <>, <, >, <=, >= or contains.
	compare struct {
		op   string
		l, r expr
	}
	// logic is l and r, l or r.
	logic struct {
		and  bool
		l, r expr
	}
	// filtered is a value followed by filters: the markup of an output or an assign.
	filtered struct {
		base    expr
		filters []filterCall
	}
)

type accessor struct {
	key   string // .key
	index expr   // [index], when key is ""
}

type filterCall struct {
	pos   int
	name  string
	args  []expr
	named map[string]expr
}

func (literal) isExpr()   {}
func (variable) isExpr()  {}
func (rangeExpr) isExpr() {}
func (compare) isExpr()   {}
func (logic) isExpr()     {}
func (filtered) isExpr()  {}

// ── Template parser ─────────────────────────────────────────────────────────────

type parser struct {
	src   string
	toks  []token
	i     int
	depth int
}

func parse(src string, start int) (*Template, error) {
	toks, err := tokenize(src, start)
	if err != nil {
		return nil, err
	}
	p := &parser{src: src, toks: toks}
	nodes, end, err := p.nodes(nil)
	if err != nil {
		return nil, err
	}
	if end != nil {
		return nil, p.unexpected(*end)
	}
	return &Template{src: src, nodes: nodes}, nil
}

// blockTags are the tags that only appear inside a block, and endTag names the end tag
// of each block tag.
var (
	blockTags = []string{"else", "elsif", "when", "endif", "endunless", "endcase", "endfor", "endcapture", "endraw", "endcomment"}
	endTag    = map[string]string{"if": "endif", "unless": "endunless", "case": "endcase", "for": "endfor", "capture": "endcapture"}
)

// nodes parses nodes up to a tag named in ends, which it returns, or to the end of the
// template (end is then nil).
func (p *parser) nodes(ends []string) (nodes []node, end *token, err error) {
	for p.i < len(p.toks) {
		t := p.toks[p.i]
		p.i++
		switch t.kind {
		case tokText:
			if t.text != "" {
				nodes = append(nodes, textNode{at{t.pos}, t.text})
			}
		case tokOutput:
			f, err := p.output(t)
			if err != nil {
				return nil, nil, err
			}
			if f != nil {
				nodes = append(nodes, outputNode{at{t.pos}, f})
			}
		case tokTag:
			if slices.Contains(ends, t.name) {
				return nodes, &t, nil
			}
			n, err := p.tag(t)
			if err != nil {
				return nil, nil, err
			}
			if n != nil {
				nodes = append(nodes, n)
			}
		}
	}
	return nodes, nil, nil
}

func (p *parser) unexpected(t token) error {
	return p.errAt(t.pos, fmt.Sprintf("unexpected {%% %s %%}", t.name))
}

func (p *parser) errAt(pos int, msg string) error { return errorAt(p.src, pos, msg) }

// block parses the body of the block tag open up to one of ends.
func (p *parser) block(open token, ends ...string) ([]node, token, error) {
	nodes, end, err := p.nodes(ends)
	if err != nil {
		return nil, token{}, err
	}
	if end == nil {
		return nil, token{}, p.errAt(open.pos,
			fmt.Sprintf("{%% %s %%} is not closed: expected {%% %s %%}", open.name, endTag[open.name]))
	}
	return nodes, *end, nil
}

func (p *parser) output(t token) (*filtered, error) {
	x, err := p.exprParser(t.markupPos, t.markupPos+len(t.markup))
	if err != nil {
		return nil, err
	}
	if x.peek().kind == xEOF {
		return nil, nil // {{ }} renders nothing
	}
	f, err := x.filtered()
	if err != nil {
		return nil, err
	}
	return f, x.end()
}

func (p *parser) tag(t token) (node, error) {
	if t.name == "" {
		return nil, p.errAt(t.pos, "expected a tag name after {%")
	}
	if slices.Contains(blockTags, t.name) {
		return nil, p.unexpected(t)
	}
	if _, ok := endTag[t.name]; ok {
		if p.depth++; p.depth > MaxDepth {
			return nil, p.errAt(t.pos, fmt.Sprintf("blocks nest more than %d deep", MaxDepth))
		}
		defer func() { p.depth-- }()
	}
	x, err := p.exprParser(t.argsPos, t.argsPos+len(t.args))
	if err != nil && t.name != "raw" && t.name != "comment" {
		return nil, err
	}
	switch t.name {
	case "assign":
		return p.assign(t, x)
	case "capture":
		return p.capture(t, x)
	case "if", "unless":
		return p.ifTag(t, x)
	case "case":
		return p.caseTag(t, x)
	case "for":
		return p.forTag(t, x)
	case "break", "continue":
		if err := x.end(); err != nil {
			return nil, err
		}
		if t.name == "break" {
			return breakNode{at{t.pos}}, nil
		}
		return continueNode{at{t.pos}}, nil
	case "raw", "comment":
		// The tokenizer put the body in one text token, followed by the end tag.
		body := p.toks[p.i]
		p.i += 2
		if t.name == "raw" && body.text != "" {
			return textNode{at{body.pos}, body.text}, nil
		}
		return nil, nil
	}
	return nil, p.errAt(t.pos, fmt.Sprintf("unknown tag %q", t.name))
}

func (p *parser) assign(t token, x *exprParser) (node, error) {
	name := x.next()
	if name.kind != xIdent {
		return nil, x.errAt(name, "expected a variable name after assign, but found "+name.describe())
	}
	if eq := x.next(); eq.kind != xAssign {
		return nil, x.errAt(eq, `expected "=" after the variable name, but found `+eq.describe())
	}
	f, err := x.filtered()
	if err != nil {
		return nil, err
	}
	if err := x.end(); err != nil {
		return nil, err
	}
	return assignNode{at: at{t.pos}, name: name.text, value: f}, nil
}

func (p *parser) capture(t token, x *exprParser) (node, error) {
	name := x.next()
	if name.kind != xIdent && name.kind != xString {
		return nil, x.errAt(name, "expected a variable name after capture, but found "+name.describe())
	}
	if err := x.end(); err != nil {
		return nil, err
	}
	body, _, err := p.block(t, "endcapture")
	if err != nil {
		return nil, err
	}
	n := name.text
	if name.kind == xString {
		n = name.str
	}
	return captureNode{at: at{t.pos}, name: n, body: body}, nil
}

func (p *parser) ifTag(t token, x *exprParser) (node, error) {
	n := ifNode{at: at{t.pos}, unless: t.name == "unless"}
	cond, err := x.condition()
	if err != nil {
		return nil, err
	}
	for {
		body, end, err := p.block(t, "elsif", "else", endTag[t.name])
		if err != nil {
			return nil, err
		}
		n.branches = append(n.branches, branch{cond: cond, body: body})
		switch end.name {
		case "elsif":
			ex, err := p.exprParser(end.argsPos, end.argsPos+len(end.args))
			if err != nil {
				return nil, err
			}
			if cond, err = ex.condition(); err != nil {
				return nil, err
			}
		case "else":
			if n.elseBody, _, err = p.block(t, endTag[t.name]); err != nil {
				return nil, err
			}
			return n, nil
		default:
			return n, nil
		}
	}
}

func (p *parser) caseTag(t token, x *exprParser) (node, error) {
	subject, err := x.primary()
	if err != nil {
		return nil, err
	}
	if err := x.end(); err != nil {
		return nil, err
	}
	n := caseNode{at: at{t.pos}, subject: subject}
	// What comes before the first when (usually white space) is never rendered.
	_, end, err := p.block(t, "when", "else", "endcase")
	if err != nil {
		return nil, err
	}
	for {
		switch end.name {
		case "when":
			ex, err := p.exprParser(end.argsPos, end.argsPos+len(end.args))
			if err != nil {
				return nil, err
			}
			var w when
			for {
				v, err := ex.primary()
				if err != nil {
					return nil, err
				}
				w.values = append(w.values, v)
				if tk := ex.peek(); tk.kind == xComma || tk.kind == xIdent && tk.text == "or" {
					ex.next()
					continue
				}
				break
			}
			if err := ex.end(); err != nil {
				return nil, err
			}
			if w.body, end, err = p.block(t, "when", "else", "endcase"); err != nil {
				return nil, err
			}
			n.whens = append(n.whens, w)
		case "else":
			if n.elseBody, _, err = p.block(t, "endcase"); err != nil {
				return nil, err
			}
			return n, nil
		default:
			return n, nil
		}
	}
}

func (p *parser) forTag(t token, x *exprParser) (node, error) {
	n := forNode{at: at{t.pos}}
	name := x.next()
	if name.kind != xIdent {
		return nil, x.errAt(name, "expected a variable name after for, but found "+name.describe())
	}
	n.name = name.text
	if in := x.next(); in.kind != xIdent || in.text != "in" {
		return nil, x.errAt(in, `expected "in" after the variable name, but found `+in.describe())
	}
	var err error
	if n.collection, err = x.primary(); err != nil {
		return nil, err
	}
	for x.peek().kind != xEOF {
		m := x.next()
		switch {
		case m.kind == xIdent && m.text == "reversed":
			n.reversed = true
		case m.kind == xIdent && (m.text == "limit" || m.text == "offset"):
			if c := x.next(); c.kind != xColon {
				return nil, x.errAt(c, fmt.Sprintf("expected \":\" after %s, but found %s", m.text, c.describe()))
			}
			v, err := x.primary()
			if err != nil {
				return nil, err
			}
			if m.text == "limit" {
				n.limit = v
			} else {
				n.offset = v
			}
		case m.kind == xComma:
		default:
			return nil, x.errAt(m, "expected limit:, offset: or reversed, but found "+m.describe())
		}
	}
	body, end, err := p.block(t, "else", "endfor")
	if err != nil {
		return nil, err
	}
	n.body = body
	if end.name == "else" {
		if n.elseBody, _, err = p.block(t, "endfor"); err != nil {
			return nil, err
		}
	}
	return n, nil
}

// ── Expression parser ───────────────────────────────────────────────────────────

type exprParser struct {
	src   string
	toks  []xtok
	i     int
	depth int
}

func (p *parser) exprParser(from, to int) (*exprParser, error) {
	toks, err := lexExpr(p.src, from, to)
	if err != nil {
		return &exprParser{src: p.src, toks: []xtok{{kind: xEOF, pos: to}}}, err
	}
	return &exprParser{src: p.src, toks: toks}, nil
}

func (x *exprParser) peek() xtok { return x.toks[x.i] }

func (x *exprParser) next() xtok {
	t := x.toks[x.i]
	if t.kind != xEOF {
		x.i++
	}
	return t
}

func (x *exprParser) errAt(t xtok, msg string) error { return errorAt(x.src, t.pos, msg) }

// end checks that all of the markup was used.
func (x *exprParser) end() error {
	if t := x.peek(); t.kind != xEOF {
		return x.errAt(t, "unexpected "+t.describe())
	}
	return nil
}

// filtered parses value | filter: arg, name: arg | ….
func (x *exprParser) filtered() (*filtered, error) {
	base, err := x.primary()
	if err != nil {
		return nil, err
	}
	f := &filtered{base: base}
	for x.peek().kind == xPipe {
		x.next()
		name := x.next()
		if name.kind != xIdent {
			return nil, x.errAt(name, `expected a filter name after "|", but found `+name.describe())
		}
		call := filterCall{pos: name.pos, name: name.text}
		if x.peek().kind == xColon {
			x.next()
			for {
				if t := x.peek(); t.kind == xIdent && x.toks[x.i+1].kind == xColon {
					x.next()
					x.next()
					v, err := x.primary()
					if err != nil {
						return nil, err
					}
					if call.named == nil {
						call.named = map[string]expr{}
					}
					call.named[t.text] = v
				} else {
					v, err := x.primary()
					if err != nil {
						return nil, err
					}
					call.args = append(call.args, v)
				}
				if x.peek().kind != xComma {
					break
				}
				x.next()
			}
		}
		f.filters = append(f.filters, call)
	}
	return f, nil
}

// condition parses comparisons joined by and/or, which Liquid evaluates right to left:
// a or b and c is a or (b and c), and a and b or c is a and (b or c).
func (x *exprParser) condition() (expr, error) {
	var operands []expr
	var ands []bool
	for {
		c, err := x.comparison()
		if err != nil {
			return nil, err
		}
		operands = append(operands, c)
		t := x.peek()
		if t.kind != xIdent || t.text != "and" && t.text != "or" {
			break
		}
		x.next()
		ands = append(ands, t.text == "and")
	}
	if err := x.end(); err != nil {
		return nil, err
	}
	e := operands[len(operands)-1]
	for k := len(operands) - 2; k >= 0; k-- {
		e = logic{and: ands[k], l: operands[k], r: e}
	}
	return e, nil
}

func (x *exprParser) comparison() (expr, error) {
	l, err := x.primary()
	if err != nil {
		return nil, err
	}
	t := x.peek()
	if t.kind != xCompare && (t.kind != xIdent || t.text != "contains") {
		if t.kind == xAssign {
			return nil, x.errAt(t, `"=" is not an operator; use "==" to compare`)
		}
		return l, nil
	}
	x.next()
	r, err := x.primary()
	if err != nil {
		return nil, err
	}
	return compare{op: t.text, l: l, r: r}, nil
}

var keywords = map[string]value{"true": true, "false": false, "nil": nil, "null": nil, "empty": emptyValue, "blank": blankValue}

// primary parses a literal, a variable with its lookups, or a range (a..b).
func (x *exprParser) primary() (expr, error) {
	t := x.next()
	switch t.kind {
	case xString:
		return literal{t.str}, nil
	case xInt:
		return literal{t.i}, nil
	case xFloat:
		return literal{t.f}, nil
	case xIdent:
		if v, ok := keywords[t.text]; ok {
			return literal{v}, nil
		}
		return x.variable(t)
	case xLParen:
		lo, err := x.rangeBound()
		if err != nil {
			return nil, err
		}
		if d := x.next(); d.kind != xDotDot {
			return nil, x.errAt(d, `expected ".." in the range, but found `+d.describe())
		}
		hi, err := x.rangeBound()
		if err != nil {
			return nil, err
		}
		if c := x.next(); c.kind != xRParen {
			return nil, x.errAt(c, `expected ")" to end the range, but found `+c.describe())
		}
		return rangeExpr{lo: lo, hi: hi}, nil
	}
	return nil, x.errAt(t, "expected a value but found "+t.describe())
}

func (x *exprParser) rangeBound() (expr, error) {
	if t := x.peek(); t.kind == xLParen {
		return nil, x.errAt(t, "a range bound must be a number or a variable")
	}
	return x.primary()
}

func (x *exprParser) variable(name xtok) (expr, error) {
	v := variable{name: name.text}
	for {
		switch x.peek().kind {
		case xDot:
			x.next()
			k := x.next()
			if k.kind != xIdent {
				return nil, x.errAt(k, `expected a name after ".", but found `+k.describe())
			}
			v.path = append(v.path, accessor{key: k.text})
		case xLBracket:
			open := x.next()
			if x.depth++; x.depth > MaxDepth {
				return nil, x.errAt(open, fmt.Sprintf("[ ] nest more than %d deep", MaxDepth))
			}
			idx, err := x.primary()
			x.depth--
			if err != nil {
				return nil, err
			}
			if c := x.next(); c.kind != xRBracket {
				return nil, x.errAt(c, `expected "]" but found `+c.describe())
			}
			v.path = append(v.path, accessor{index: idx})
		default:
			return v, nil
		}
	}
}
