package liquid

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ── Template tokens ─────────────────────────────────────────────────────────────

type tokKind int

const (
	tokText tokKind = iota
	tokOutput
	tokTag
)

// token is a piece of the template: text, an output {{ … }} or a tag {% … %}.
type token struct {
	kind tokKind
	pos  int    // offset of the token's start ("{{", "{%" or the text)
	text string // text: the text itself

	// Output and tag: the markup between the delimiters, without the trim dashes.
	markup    string
	markupPos int
	// Tag: its name and the markup after it.
	name    string
	args    string
	argsPos int

	trimLeft, trimRight bool // {{- / {%- and -}} / -%}
}

const whitespace = " \t\n\r\f\v"

var (
	endRawRe     = regexp.MustCompile(`\{%-?\s*endraw\s*-?%\}`)
	commentTagRe = regexp.MustCompile(`\{%-?\s*(end)?comment(?:\s[^%]*)?-?%\}`)
)

// tokenize splits src, from offset start, into tokens and applies the whitespace
// control dashes. The body of raw and comment is a single text token, whatever it
// contains.
func tokenize(src string, start int) ([]token, error) {
	var toks []token
	i := start
	for i < len(src) {
		j := indexOpen(src, i)
		if j < 0 {
			toks = append(toks, token{kind: tokText, pos: i, text: src[i:]})
			break
		}
		if j > i {
			toks = append(toks, token{kind: tokText, pos: i, text: src[i:j]})
		}
		tok, end, err := delimited(src, j)
		if err != nil {
			return nil, err
		}
		toks = append(toks, tok)
		i = end
		if tok.kind == tokTag && (tok.name == "raw" || tok.name == "comment") {
			body, closing, next, err := rawBody(src, tok, end)
			if err != nil {
				return nil, err
			}
			toks = append(toks, body, closing)
			i = next
		}
	}
	for k, t := range toks {
		if t.kind == tokText {
			continue
		}
		if t.trimLeft && k > 0 && toks[k-1].kind == tokText {
			toks[k-1].text = strings.TrimRight(toks[k-1].text, whitespace)
		}
		if t.trimRight && k+1 < len(toks) && toks[k+1].kind == tokText {
			toks[k+1].text = strings.TrimLeft(toks[k+1].text, whitespace)
		}
	}
	return toks, nil
}

// indexOpen returns the offset of the next "{{" or "{%" at or after i, or -1.
func indexOpen(src string, i int) int {
	for {
		k := strings.IndexByte(src[i:], '{')
		if k < 0 || i+k+1 >= len(src) {
			return -1
		}
		i += k
		if c := src[i+1]; c == '{' || c == '%' {
			return i
		}
		i++
	}
}

// delimited reads the output or tag that starts at offset j, and returns it with the
// offset after it. The closing delimiter is looked for outside quoted strings.
func delimited(src string, j int) (token, int, error) {
	output := src[j+1] == '{'
	closer, what := "%}", `"{%"`
	tok := token{kind: tokTag, pos: j}
	if output {
		closer, what = "}}", `"{{"`
		tok.kind = tokOutput
	}
	from := j + 2
	if from < len(src) && src[from] == '-' {
		tok.trimLeft = true
		from++
	}
	quoted := true
	if !output {
		// raw and comment (and their end tags) hold free text, where a lone ' is fine.
		rest := strings.TrimLeft(src[from:], whitespace)
		n := 0
		for n < len(rest) && isIdentByte(rest[n]) {
			n++
		}
		switch rest[:n] {
		case "raw", "endraw", "comment", "endcomment":
			quoted = false
		}
	}
	end := closeIndex(src, from, closer, quoted)
	if end < 0 {
		return token{}, 0, errorAt(src, j, fmt.Sprintf("%s is not closed: expected %q", what, closer))
	}
	to := end
	if to > from && src[to-1] == '-' {
		tok.trimRight = true
		to--
	}
	tok.markup, tok.markupPos = src[from:to], from
	if !output {
		lead := len(tok.markup) - len(strings.TrimLeft(tok.markup, whitespace))
		rest := tok.markup[lead:]
		n := 0
		for n < len(rest) && isIdentByte(rest[n]) {
			n++
		}
		tok.name = rest[:n]
		tok.args, tok.argsPos = rest[n:], from+lead+n
	}
	return tok, end + 2, nil
}

// closeIndex returns the offset of closer at or after i, or -1; when quoted, outside
// '…' and "…".
func closeIndex(src string, i int, closer string, quoted bool) int {
	if !quoted {
		if k := strings.Index(src[i:], closer); k >= 0 {
			return i + k
		}
		return -1
	}
	var quote byte
	for ; i < len(src); i++ {
		c := src[i]
		switch {
		case quote != 0:
			if c == '\\' {
				i++ // an escaped character, possibly the quote
			} else if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
		case strings.HasPrefix(src[i:], closer):
			return i
		}
	}
	return -1
}

// rawBody reads the body of a raw or comment tag that ends at offset from: a text token
// with everything up to the matching end tag, the end tag, and the offset after it.
// Comments nest; raw does not.
func rawBody(src string, open token, from int) (token, token, int, error) {
	var loc []int
	if open.name == "raw" {
		loc = endRawRe.FindStringIndex(src[from:])
	} else {
		depth := 1
		for _, m := range commentTagRe.FindAllStringSubmatchIndex(src[from:], -1) {
			if m[2] < 0 {
				depth++
				continue
			}
			if depth--; depth == 0 {
				loc = m[:2]
				break
			}
		}
	}
	if loc == nil {
		return token{}, token{}, 0, errorAt(src, open.pos,
			fmt.Sprintf("{%% %s %%} is not closed: expected {%% end%s %%}", open.name, open.name))
	}
	start, end := from+loc[0], from+loc[1]
	closing, _, err := delimited(src, start)
	if err != nil {
		return token{}, token{}, 0, err
	}
	return token{kind: tokText, pos: from, text: src[from:start]}, closing, end, nil
}

func isIdentByte(c byte) bool {
	return c == '_' || c == '-' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

func isIdentStart(c byte) bool {
	return c == '_' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// ── Expression tokens ───────────────────────────────────────────────────────────

type xkind int

const (
	xEOF xkind = iota
	xIdent
	xString
	xInt
	xFloat
	xCompare // == != <> < > <= >=
	xAssign  // =
	xPipe
	xColon
	xComma
	xDot
	xDotDot
	xLBracket
	xRBracket
	xLParen
	xRParen
)

type xtok struct {
	kind xkind
	pos  int
	text string // as written
	str  string // xString: the value
	i    int64
	f    float64
}

func (t xtok) describe() string {
	if t.kind == xEOF {
		return "nothing"
	}
	return strconv.Quote(t.text)
}

// lexExpr splits the markup src[from:to] into expression tokens, ending with xEOF.
func lexExpr(src string, from, to int) ([]xtok, error) {
	var toks []xtok
	i := from
	for {
		for i < to && strings.IndexByte(whitespace, src[i]) >= 0 {
			i++
		}
		if i >= to {
			return append(toks, xtok{kind: xEOF, pos: to}), nil
		}
		c := src[i]
		start := i
		switch {
		case isIdentStart(c):
			for i < to && isIdentByte(src[i]) {
				i++
			}
			if i < to && src[i] == '?' {
				i++
			}
			toks = append(toks, xtok{kind: xIdent, pos: start, text: src[start:i]})
		case isDigit(c) || c == '-' && i+1 < to && isDigit(src[i+1]):
			t, err := lexNumber(src, i, to)
			if err != nil {
				return nil, err
			}
			toks = append(toks, t)
			i += len(t.text)
		case c == '\'' || c == '"':
			t, err := lexString(src, i, to)
			if err != nil {
				return nil, err
			}
			toks = append(toks, t)
			i += len(t.text)
		case c == '=' || c == '!' || c == '<' || c == '>':
			n := 1
			if i+1 < to {
				two := src[i : i+2]
				if two == "==" || two == "!=" || two == "<>" || two == "<=" || two == ">=" {
					n = 2
				}
			}
			kind := xCompare
			if n == 1 && c == '=' {
				kind = xAssign
			} else if n == 1 && c == '!' {
				return nil, errorAt(src, i, `unexpected "!": use "!=" to compare, or unless for "not"`)
			}
			i += n
			toks = append(toks, xtok{kind: kind, pos: start, text: src[start:i]})
		case c == '.':
			kind := xDot
			i++
			if i < to && src[i] == '.' {
				kind = xDotDot
				i++
			}
			toks = append(toks, xtok{kind: kind, pos: start, text: src[start:i]})
		default:
			kinds := map[byte]xkind{'|': xPipe, ':': xColon, ',': xComma, '[': xLBracket, ']': xRBracket, '(': xLParen, ')': xRParen}
			kind, ok := kinds[c]
			if !ok {
				r, _ := utf8.DecodeRuneInString(src[i:])
				return nil, errorAt(src, i, fmt.Sprintf("unexpected character %q", r))
			}
			i++
			toks = append(toks, xtok{kind: kind, pos: start, text: src[start:i]})
		}
	}
}

func isDigit(c byte) bool { return '0' <= c && c <= '9' }

// lexNumber reads -?digits(.digits)?. A "." not followed by a digit (as in 1..5) is
// not part of the number.
func lexNumber(src string, i, to int) (xtok, error) {
	start := i
	if src[i] == '-' {
		i++
	}
	for i < to && isDigit(src[i]) {
		i++
	}
	isFloat := false
	if i+1 < to && src[i] == '.' && isDigit(src[i+1]) {
		isFloat = true
		i++
		for i < to && isDigit(src[i]) {
			i++
		}
	}
	text := src[start:i]
	if isFloat {
		f, err := strconv.ParseFloat(text, 64)
		if err != nil {
			return xtok{}, errorAt(src, start, fmt.Sprintf("number %s is out of range", text))
		}
		return xtok{kind: xFloat, pos: start, text: text, f: f}, nil
	}
	n, err := strconv.ParseInt(text, 10, 64)
	if err != nil {
		return xtok{}, errorAt(src, start, fmt.Sprintf("number %s is out of range", text))
	}
	return xtok{kind: xInt, pos: start, text: text, i: n}, nil
}

// lexString reads a quoted string. A backslash escapes the next character, with \n, \r
// and \t meaning a line feed, a carriage return and a tab, as in LiquidJS.
func lexString(src string, i, to int) (xtok, error) {
	quote := src[i]
	start := i
	i++
	var b strings.Builder
	for i < to {
		c := src[i]
		switch {
		case c == quote:
			return xtok{kind: xString, pos: start, text: src[start : i+1], str: b.String()}, nil
		case c == '\\' && i+1 < to:
			i++
			switch e := src[i]; e {
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 't':
				b.WriteByte('\t')
			default:
				b.WriteByte(e)
			}
		default:
			b.WriteByte(c)
		}
		i++
	}
	return xtok{}, errorAt(src, start, "the string is not closed")
}
