// Package builder is the logic of the client condition builder (spec §10.5): a condition
// as an editable tree, the form it is posted as, and the edits the builder offers. The UI
// renders the tree, posts the whole form with one edit, and renders the result; nothing
// is saved until Save, which turns the tree back into a condition.
//
// The tree's root is always a group, so it can be added to and never removed. Each node
// is named by its path: the root is "n.0", its children "n.0.0", "n.0.1", …, and its
// form inputs are "<path>.kind", "<path>.not", "<path>.op" (groups), "<path>.header" and
// "<path>.value" (items), for example n.0.1.kind=ip, n.0.1.value=10.0.0.1.
//
// The package is backend code: standard library only.
package builder

import (
	"errors"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"

	"github.com/laposa/tmock/app-go/internal/condition"
)

// Kind is what a node is: a group of nodes or one of the four predicates.
type Kind string

// The kinds, as the form names them.
const (
	Group       Kind = "group"
	IP          Kind = "ip"
	CIDR        Kind = "cidr"
	Header      Kind = "header"      // headerMatch
	HeaderRegex Kind = "headerRegex" // headerRegex
)

// Kinds are the kinds the "Add Expression" menu offers, in its order.
var Kinds = []Kind{Group, IP, CIDR, Header, HeaderRegex}

// Label is the kind's name in the builder ("IP Address").
func (k Kind) Label() string {
	switch k {
	case Group:
		return "Group"
	case IP:
		return "IP Address"
	case CIDR:
		return "CIDR"
	case Header:
		return "Header"
	case HeaderRegex:
		return "Header Regex"
	}
	return string(k)
}

// HasHeader reports whether items of the kind have a header name.
func (k Kind) HasHeader() bool { return k == Header || k == HeaderRegex }

// Node is a node of the tree: a group (Or false is AND) with its children, or an item
// with its header name (Header and HeaderRegex) and value. Not negates either.
type Node struct {
	Kind     Kind
	Not      bool
	Or       bool
	Header   string
	Value    string
	Children []Node
}

// Root is the path of the root group.
const Root = "n.0"

// Limits on a posted tree, so a crafted form cannot make the server work hard.
const (
	MaxNodes = 500
	MaxDepth = 32
)

// ErrForm wraps the reasons a posted form is not a tree.
var ErrForm = errors.New("not a valid condition form")

// FromCondition is the tree of c. No condition is an empty AND group (spec §10.5); a
// condition whose root is an item becomes an AND group with that item, which matches
// the same requests.
func FromCondition(c *condition.Condition) Node {
	if c == nil {
		return Node{Kind: Group}
	}
	n := fromCondition(*c)
	if n.Kind != Group {
		return Node{Kind: Group, Children: []Node{n}}
	}
	return n
}

func fromCondition(c condition.Condition) Node {
	n := Node{Not: c.Not}
	switch {
	case c.And != nil || c.Or != nil:
		n.Kind = Group
		list := c.And
		if c.And == nil {
			n.Or, list = true, c.Or
		}
		for _, ch := range list {
			n.Children = append(n.Children, fromCondition(ch))
		}
	case c.HeaderMatch != nil:
		n.Kind, n.Header, n.Value = Header, c.HeaderMatch.Header, c.HeaderMatch.Value
	case c.HeaderRegex != nil:
		n.Kind, n.Header, n.Value = HeaderRegex, c.HeaderRegex.Header, c.HeaderRegex.Value
	case c.IP != nil:
		n.Kind, n.Value = IP, *c.IP
	case c.CIDR != nil:
		n.Kind, n.Value = CIDR, *c.CIDR
	default: // no predicate: only in a condition that failed validation
		n.Kind = Group
	}
	return n
}

// Condition is the condition the tree stands for. A root group without children is no
// condition (nil), whatever its NOT and operator, so an emptied builder saves null.
// Header names, IP addresses and CIDR prefixes are trimmed; header values and patterns
// are kept as typed.
func (n Node) Condition() *condition.Condition {
	if n.Kind == Group && len(n.Children) == 0 {
		return nil
	}
	c := n.condition()
	return &c
}

func (n Node) condition() condition.Condition {
	c := condition.Condition{Not: n.Not}
	switch n.Kind {
	case Group:
		list := make([]condition.Condition, 0, len(n.Children))
		for _, ch := range n.Children {
			list = append(list, ch.condition())
		}
		if n.Or {
			c.Or = list
		} else {
			c.And = list
		}
	case Header:
		c.HeaderMatch = &condition.Header{Header: strings.TrimSpace(n.Header), Value: n.Value}
	case HeaderRegex:
		c.HeaderRegex = &condition.Header{Header: strings.TrimSpace(n.Header), Value: n.Value}
	case IP:
		v := strings.TrimSpace(n.Value)
		c.IP = &v
	case CIDR:
		v := strings.TrimSpace(n.Value)
		c.CIDR = &v
	}
	return c
}

// Form is the tree as the builder's form posts it.
func (n Node) Form() url.Values {
	v := url.Values{}
	n.form(Root, v)
	return v
}

func (n Node) form(path string, v url.Values) {
	v.Set(path+".kind", string(n.Kind))
	if n.Not {
		v.Set(path+".not", "on")
	}
	if n.Kind == Group {
		op := "and"
		if n.Or {
			op = "or"
		}
		v.Set(path+".op", op)
		for i, ch := range n.Children {
			ch.form(Child(path, i), v)
		}
		return
	}
	if n.Kind.HasHeader() {
		v.Set(path+".header", n.Header)
	}
	v.Set(path+".value", n.Value)
}

// Child is the path of the i-th child of the node at path.
func Child(path string, i int) string { return path + "." + strconv.Itoa(i) }

// ParseForm reads the tree from the builder's form; inputs not under "n." are ignored.
// Children are ordered by their index, which need not be contiguous. Every node needs a
// known kind, every path a parent, and the tree must keep within MaxNodes and MaxDepth.
func ParseForm(form url.Values) (Node, error) {
	type fields map[string]string
	nodes := map[string]fields{}
	for key, vals := range form {
		if !strings.HasPrefix(key, "n.") {
			continue
		}
		dot := strings.LastIndexByte(key, '.')
		path, field := key[:dot], key[dot+1:]
		switch field {
		case "kind", "not", "op", "header", "value":
		default:
			return Node{}, fmt.Errorf("%w: unknown input %q", ErrForm, key)
		}
		if err := checkPath(path); err != nil {
			return Node{}, err
		}
		if nodes[path] == nil {
			if len(nodes) == MaxNodes {
				return Node{}, fmt.Errorf("%w: more than %d expressions", ErrForm, MaxNodes)
			}
			nodes[path] = fields{}
		}
		if len(vals) > 0 {
			nodes[path][field] = vals[0]
		}
	}
	children := map[string][]string{}
	for path := range nodes {
		if path == Root {
			continue
		}
		parent := path[:strings.LastIndexByte(path, '.')]
		if nodes[parent] == nil {
			return Node{}, fmt.Errorf("%w: %s has no parent", ErrForm, path)
		}
		children[parent] = append(children[parent], path)
	}
	if nodes[Root] == nil {
		nodes[Root] = fields{"kind": string(Group)}
	}

	var build func(path string) (Node, error)
	build = func(path string) (Node, error) {
		f := nodes[path]
		n := Node{Kind: Kind(f["kind"]), Not: f["not"] != ""}
		switch n.Kind {
		case Group:
			switch f["op"] {
			case "", "and":
			case "or":
				n.Or = true
			default:
				return Node{}, fmt.Errorf("%w: %s.op must be and or or", ErrForm, path)
			}
		case IP, CIDR, Header, HeaderRegex:
			if len(children[path]) > 0 {
				return Node{}, fmt.Errorf("%w: %s is not a group", ErrForm, path)
			}
			n.Value = f["value"]
			if n.Kind.HasHeader() {
				n.Header = f["header"]
			}
			return n, nil
		default:
			return Node{}, fmt.Errorf("%w: %s.kind is missing or unknown", ErrForm, path)
		}
		kids := children[path]
		sort.Slice(kids, func(i, j int) bool { return lastIndex(kids[i]) < lastIndex(kids[j]) })
		for _, k := range kids {
			ch, err := build(k)
			if err != nil {
				return Node{}, err
			}
			n.Children = append(n.Children, ch)
		}
		return n, nil
	}
	root, err := build(Root)
	if err != nil {
		return Node{}, err
	}
	if root.Kind != Group {
		return Node{}, fmt.Errorf("%w: the root must be a group", ErrForm)
	}
	return root, nil
}

// checkPath accepts "n.0" and paths below it with canonical indexes ("n.0.12").
func checkPath(path string) error {
	segs := strings.Split(path, ".")
	if len(segs) < 2 || segs[0] != "n" || segs[1] != "0" {
		return fmt.Errorf("%w: bad path %q", ErrForm, path)
	}
	if len(segs)-1 > MaxDepth {
		return fmt.Errorf("%w: nested deeper than %d", ErrForm, MaxDepth)
	}
	for _, s := range segs[2:] {
		if i, err := strconv.Atoi(s); err != nil || i < 0 || strconv.Itoa(i) != s || i >= MaxNodes {
			return fmt.Errorf("%w: bad path %q", ErrForm, path)
		}
	}
	return nil
}

func lastIndex(path string) int {
	i, _ := strconv.Atoi(path[strings.LastIndexByte(path, '.')+1:])
	return i
}

// Op is an edit of the tree (spec §10.5).
type Op string

// The edits. AddGroup … AddHeaderRegex append a new node to a group.
const (
	AddGroup       Op = "add-group"
	AddIP          Op = "add-ip"
	AddCIDR        Op = "add-cidr"
	AddHeader      Op = "add-header"
	AddHeaderRegex Op = "add-headerRegex"
	Remove         Op = "remove" // a node other than the root
	ToggleNot      Op = "not"    // NOT on a group or an item
	ToggleAndOr    Op = "and-or" // AND ↔ OR of a group
)

// AddOp is the edit that adds a node of kind k.
func AddOp(k Kind) Op { return Op("add-" + string(k)) }

// Edit is an edit and the path of the node it applies to, as the builder's buttons send
// it: "<op>:<path>", for example "remove:n.0.1".
type Edit struct {
	Op   Op
	Path string
}

// String is the edit as a button's value.
func (e Edit) String() string { return string(e.Op) + ":" + e.Path }

// ParseEdit reads an edit written by Edit.String.
func ParseEdit(s string) (Edit, error) {
	op, path, ok := strings.Cut(s, ":")
	if !ok {
		return Edit{}, fmt.Errorf("%w: bad edit %q", ErrForm, s)
	}
	if err := checkPath(path); err != nil {
		return Edit{}, err
	}
	return Edit{Op: Op(op), Path: path}, nil
}

// Apply makes the edit on the tree at root. A path that names no node, an add or AND/OR
// on an item, removing the root and an unknown edit are errors, and leave the tree as
// it was.
func Apply(root *Node, e Edit) error {
	if e.Path == Root && e.Op == Remove {
		return fmt.Errorf("%w: the top-level group cannot be removed", ErrForm)
	}
	parent, idx, n := find(root, e.Path)
	if n == nil {
		return fmt.Errorf("%w: no expression at %s", ErrForm, e.Path)
	}
	switch e.Op {
	case AddGroup, AddIP, AddCIDR, AddHeader, AddHeaderRegex:
		if n.Kind != Group {
			return fmt.Errorf("%w: %s is not a group", ErrForm, e.Path)
		}
		if count(*root) >= MaxNodes {
			return fmt.Errorf("%w: more than %d expressions", ErrForm, MaxNodes)
		}
		if depth(e.Path) >= MaxDepth {
			return fmt.Errorf("%w: nested deeper than %d", ErrForm, MaxDepth)
		}
		n.Children = append(n.Children, Node{Kind: Kind(strings.TrimPrefix(string(e.Op), "add-"))})
	case Remove:
		parent.Children = append(parent.Children[:idx:idx], parent.Children[idx+1:]...)
	case ToggleNot:
		n.Not = !n.Not
	case ToggleAndOr:
		if n.Kind != Group {
			return fmt.Errorf("%w: %s is not a group", ErrForm, e.Path)
		}
		n.Or = !n.Or
	default:
		return fmt.Errorf("%w: unknown edit %q", ErrForm, e.Op)
	}
	return nil
}

// find returns the node at path, with its parent and its index there (nil and 0 for the
// root), or a nil node.
func find(root *Node, path string) (parent *Node, idx int, n *Node) {
	if !strings.HasPrefix(path+".", Root+".") {
		return nil, 0, nil
	}
	n = root
	rest := strings.TrimPrefix(path, Root)
	for rest != "" {
		seg, tail, _ := strings.Cut(rest[1:], ".")
		i, err := strconv.Atoi(seg)
		if err != nil || i < 0 || i >= len(n.Children) {
			return nil, 0, nil
		}
		parent, idx, n = n, i, &n.Children[i]
		if tail == "" {
			break
		}
		rest = "." + tail
	}
	return parent, idx, n
}

func count(n Node) int {
	c := 1
	for _, ch := range n.Children {
		c += count(ch)
	}
	return c
}

func depth(path string) int { return strings.Count(path, ".") }

// FieldOf maps the path of a condition.Problem ("or[1].headerMatch.value") to the
// builder: the path of the node it is about ("n.0.1") and its input ("value", "header",
// or "" for the node itself). The mapping follows Condition, so it holds for problems
// found in the condition the tree returned.
func FieldOf(problem string) (path, field string) {
	path, rest := Root, problem
	for rest != "" {
		seg, tail, _ := strings.Cut(rest, ".")
		if open := strings.IndexByte(seg, '['); open > 0 && strings.HasSuffix(seg, "]") && (seg[:open] == "and" || seg[:open] == "or") {
			path += "." + seg[open+1:len(seg)-1]
			rest = tail
			continue
		}
		switch seg {
		case "ip", "cidr":
			return path, "value"
		case "headerMatch", "headerRegex":
			if tail == "header" {
				return path, "header"
			}
			return path, "value"
		}
		return path, ""
	}
	return path, ""
}
