package builder

import (
	"encoding/json"
	"errors"
	"math/rand/v2"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/condition"
)

func parse(t *testing.T, s string) *condition.Condition {
	t.Helper()
	c, err := condition.Parse(s)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func toJSON(t *testing.T, c *condition.Condition) string {
	t.Helper()
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// The example of spec §15.3 I08: NOT (ip = 10.0.0.1 OR (header X-Env = qa AND cidr
// 192.168.0.0/16)), built with the edits, saved, and reopened as the same tree.
func TestBuildTheSpecExample(t *testing.T) {
	root := FromCondition(nil)
	if !reflect.DeepEqual(root, Node{Kind: Group}) {
		t.Fatalf("an empty condition starts as an empty AND group: %+v", root)
	}
	steps := []Edit{
		{ToggleNot, Root},
		{ToggleAndOr, Root},
		{AddIP, Root},
		{AddGroup, Root},
		{AddHeader, "n.0.1"},
		{AddCIDR, "n.0.1"},
	}
	for _, e := range steps {
		if err := Apply(&root, e); err != nil {
			t.Fatalf("%v: %v", e, err)
		}
	}
	root.Children[0].Value = "10.0.0.1"
	root.Children[1].Children[0].Header, root.Children[1].Children[0].Value = "X-Env", "qa"
	root.Children[1].Children[1].Value = "192.168.0.0/16"

	want := `{"or":[{"ip":"10.0.0.1"},{"and":[{"headerMatch":{"header":"X-Env","value":"qa"}},{"cidr":"192.168.0.0/16"}]}],"not":true}`
	c := root.Condition()
	if got := toJSON(t, c); got != want {
		t.Fatalf("saved:\n got %s\nwant %s", got, want)
	}
	if p := condition.Validate(c); len(p) != 0 {
		t.Errorf("valid condition reported: %v", p)
	}
	if back := FromCondition(parse(t, want)); !reflect.DeepEqual(back, root) {
		t.Errorf("reopened:\n got %+v\nwant %+v", back, root)
	}
	// The form the builder posts gives the same tree.
	if back, err := ParseForm(root.Form()); err != nil || !reflect.DeepEqual(back, root) {
		t.Errorf("form round-trip: %+v, %v", back, err)
	}
}

func TestEdits(t *testing.T) {
	for _, k := range Kinds {
		root := Node{Kind: Group}
		if err := Apply(&root, Edit{AddOp(k), Root}); err != nil {
			t.Fatalf("add %s: %v", k, err)
		}
		// A new group is an empty AND group; a new item has empty values.
		if len(root.Children) != 1 || !reflect.DeepEqual(root.Children[0], Node{Kind: k}) {
			t.Errorf("add %s: %+v", k, root.Children)
		}
	}

	root := Node{Kind: Group, Children: []Node{{Kind: IP, Value: "a"}, {Kind: CIDR, Value: "b"}, {Kind: Header, Header: "h", Value: "c"}}}
	if err := Apply(&root, Edit{Remove, "n.0.1"}); err != nil {
		t.Fatal(err)
	}
	if len(root.Children) != 2 || root.Children[0].Value != "a" || root.Children[1].Value != "c" {
		t.Errorf("remove the middle child: %+v", root.Children)
	}
	if err := Apply(&root, Edit{ToggleNot, "n.0.1"}); err != nil || !root.Children[1].Not {
		t.Errorf("NOT on an item: %+v, %v", root.Children[1], err)
	}
	if err := Apply(&root, Edit{ToggleNot, "n.0.1"}); err != nil || root.Children[1].Not {
		t.Errorf("NOT toggles back: %+v, %v", root.Children[1], err)
	}
	if err := Apply(&root, Edit{ToggleAndOr, Root}); err != nil || !root.Or {
		t.Errorf("AND → OR: %+v, %v", root, err)
	}
	if err := Apply(&root, Edit{ToggleAndOr, Root}); err != nil || root.Or {
		t.Errorf("OR → AND: %+v, %v", root, err)
	}

	// Edits that do not apply leave the tree as it was.
	before := root
	before.Children = append([]Node(nil), root.Children...)
	for _, e := range []Edit{
		{Remove, Root},
		{AddIP, "n.0.0"},       // an item
		{ToggleAndOr, "n.0.0"}, // an item
		{ToggleNot, "n.0.5"},   // no such node
		{Remove, "n.0.0.0"},    // below an item
		{"explode", Root},
	} {
		if err := Apply(&root, e); !errors.Is(err, ErrForm) {
			t.Errorf("%v: want ErrForm, got %v", e, err)
		}
	}
	if !reflect.DeepEqual(root, before) {
		t.Errorf("a failed edit changed the tree: %+v", root)
	}
}

func TestEditLimits(t *testing.T) {
	root := Node{Kind: Group}
	path := Root
	for range MaxDepth - 1 {
		if err := Apply(&root, Edit{AddGroup, path}); err != nil {
			t.Fatal(err)
		}
		path = Child(path, 0)
	}
	if err := Apply(&root, Edit{AddGroup, path}); !errors.Is(err, ErrForm) {
		t.Errorf("deeper than MaxDepth: %v", err)
	}
	wide := Node{Kind: Group, Children: make([]Node, MaxNodes-1)}
	for i := range wide.Children {
		wide.Children[i] = Node{Kind: IP}
	}
	if err := Apply(&wide, Edit{AddIP, Root}); !errors.Is(err, ErrForm) {
		t.Errorf("more than MaxNodes: %v", err)
	}
}

func TestParseEdit(t *testing.T) {
	e := Edit{Remove, "n.0.3.1"}
	if got, err := ParseEdit(e.String()); err != nil || got != e {
		t.Errorf("round-trip: %v, %v", got, err)
	}
	for _, s := range []string{"", "remove", "remove:", "remove:n.1", "remove:x.0", "remove:n.0.01", "remove:n.0.-1", "remove:n.0.a"} {
		if _, err := ParseEdit(s); !errors.Is(err, ErrForm) {
			t.Errorf("%q: want ErrForm, got %v", s, err)
		}
	}
}

func TestParseForm(t *testing.T) {
	// Indexes need not be contiguous: removed rows leave gaps until the next render.
	form := url.Values{
		"name":          {"ignored"},
		"n.0.kind":      {"group"},
		"n.0.op":        {"or"},
		"n.0.7.kind":    {"ip"},
		"n.0.7.value":   {"10.0.0.1"},
		"n.0.2.kind":    {"headerRegex"},
		"n.0.2.header":  {"X-A"},
		"n.0.2.value":   {"^a"},
		"n.0.2.not":     {"on"},
		"n.0.10.kind":   {"group"},
		"n.0.10.0.kind": {"cidr"},
	}
	got, err := ParseForm(form)
	want := Node{Kind: Group, Or: true, Children: []Node{
		{Kind: HeaderRegex, Not: true, Header: "X-A", Value: "^a"},
		{Kind: IP, Value: "10.0.0.1"},
		{Kind: Group, Children: []Node{{Kind: CIDR}}},
	}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Errorf("parse:\n got %+v, %v\nwant %+v", got, err, want)
	}
	// A form without the builder's inputs is an empty AND group.
	if got, err := ParseForm(url.Values{"name": {"x"}}); err != nil || !reflect.DeepEqual(got, Node{Kind: Group}) {
		t.Errorf("empty form: %+v, %v", got, err)
	}

	bad := map[string]url.Values{
		"unknown input":     {"n.0.kind": {"group"}, "n.0.colour": {"red"}},
		"unknown kind":      {"n.0.kind": {"group"}, "n.0.0.kind": {"port"}},
		"missing kind":      {"n.0.kind": {"group"}, "n.0.0.value": {"x"}},
		"orphan":            {"n.0.kind": {"group"}, "n.0.3.1.kind": {"ip"}},
		"item with child":   {"n.0.kind": {"group"}, "n.0.0.kind": {"ip"}, "n.0.0.0.kind": {"ip"}},
		"root is an item":   {"n.0.kind": {"ip"}},
		"bad op":            {"n.0.kind": {"group"}, "n.0.op": {"xor"}},
		"second root":       {"n.1.kind": {"group"}},
		"non-canonical idx": {"n.0.kind": {"group"}, "n.0.01.kind": {"ip"}},
		"too deep":          {"n.0" + strings.Repeat(".0", MaxDepth) + ".kind": {"ip"}},
	}
	for name, f := range bad {
		if _, err := ParseForm(f); !errors.Is(err, ErrForm) {
			t.Errorf("%s: want ErrForm, got %v", name, err)
		}
	}
	many := url.Values{"n.0.kind": {"group"}}
	for i := range MaxNodes {
		many.Set("n.0."+strconv.Itoa(i)+".kind", "ip")
	}
	if _, err := ParseForm(many); !errors.Is(err, ErrForm) {
		t.Errorf("more than MaxNodes: %v", err)
	}
}

func TestConditionAndFromCondition(t *testing.T) {
	// An emptied builder saves no condition, whatever the root's NOT and operator.
	for _, n := range []Node{{Kind: Group}, {Kind: Group, Not: true, Or: true}} {
		if c := n.Condition(); c != nil {
			t.Errorf("%+v: want nil, got %s", n, toJSON(t, c))
		}
	}
	// A nested empty group is kept as [].
	n := Node{Kind: Group, Children: []Node{{Kind: Group, Or: true}}}
	if got := toJSON(t, n.Condition()); got != `{"and":[{"or":[]}]}` {
		t.Errorf("nested empty group: %s", got)
	}
	// Trimming: header names, IPs and CIDRs; header values and patterns stay as typed.
	n = Node{Kind: Group, Children: []Node{
		{Kind: IP, Value: " 10.0.0.1 "}, {Kind: CIDR, Value: " ::/0"}, {Kind: Header, Header: " X-A ", Value: " v "},
	}}
	if got := toJSON(t, n.Condition()); got != `{"and":[{"ip":"10.0.0.1"},{"cidr":"::/0"},{"headerMatch":{"header":"X-A","value":" v "}}]}` {
		t.Errorf("trimming: %s", got)
	}
	// A condition whose root is an item opens as an AND group with that item.
	got := FromCondition(parse(t, `{"not":true,"ip":"1.2.3.4"}`))
	if want := (Node{Kind: Group, Children: []Node{{Kind: IP, Not: true, Value: "1.2.3.4"}}}); !reflect.DeepEqual(got, want) {
		t.Errorf("item root: %+v", got)
	}
}

func TestFieldOf(t *testing.T) {
	for problem, want := range map[string][2]string{
		"":                               {Root, ""},
		"ip":                             {Root, "value"},
		"and[0]":                         {"n.0.0", ""},
		"or[1].cidr":                     {"n.0.1", "value"},
		"and[2].or[0].headerMatch.value": {"n.0.2.0", "value"},
		"and[0].headerRegex.header":      {"n.0.0", "header"},
		"or[3].headerRegex.value":        {"n.0.3", "value"},
	} {
		path, field := FieldOf(problem)
		if path != want[0] || field != want[1] {
			t.Errorf("%q: got %s %q, want %s %q", problem, path, field, want[0], want[1])
		}
	}
	// FieldOf points at the node whose value is wrong, for a tree built in the builder.
	root := Node{Kind: Group, Or: true, Children: []Node{
		{Kind: IP, Value: "1.2.3.4"},
		{Kind: Group, Children: []Node{{Kind: Header, Header: "X", Value: "1"}, {Kind: CIDR, Value: "nope"}}},
	}}
	problems := condition.Validate(root.Condition())
	if len(problems) != 1 {
		t.Fatalf("problems: %v", problems)
	}
	if path, field := FieldOf(problems[0].Path); path != "n.0.1.1" || field != "value" {
		t.Errorf("%s → %s %q", problems[0].Path, path, field)
	}
}

// Random trees survive both round trips: form → tree, and tree → condition JSON → tree.
func TestRoundTripRandomTrees(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for i := range 2000 {
		root := randomGroup(rng, 0)
		if len(root.Children) == 0 {
			root.Children = []Node{{Kind: IP, Value: "10.0.0.1"}} // an empty root saves null
		}
		back, err := ParseForm(root.Form())
		if err != nil || !reflect.DeepEqual(back, root) {
			t.Fatalf("tree %d, form round-trip:\n got %+v, %v\nwant %+v", i, back, err, root)
		}
		text := toJSON(t, root.Condition())
		reopened := FromCondition(parse(t, text))
		if !reflect.DeepEqual(reopened, root) {
			t.Fatalf("tree %d, JSON round-trip via %s:\n got %+v\nwant %+v", i, text, reopened, root)
		}
	}
}

func randomGroup(rng *rand.Rand, depth int) Node {
	n := Node{Kind: Group, Not: rng.IntN(3) == 0, Or: rng.IntN(2) == 0}
	kids := rng.IntN(4)
	for range kids {
		if depth < 4 && rng.IntN(3) == 0 {
			n.Children = append(n.Children, randomGroup(rng, depth+1))
			continue
		}
		item := Node{Kind: Kinds[1+rng.IntN(len(Kinds)-1)], Not: rng.IntN(3) == 0, Value: "v" + strconv.Itoa(rng.IntN(100))}
		if item.Kind.HasHeader() {
			item.Header = "X-" + strconv.Itoa(rng.IntN(10))
		}
		n.Children = append(n.Children, item)
	}
	return n
}
