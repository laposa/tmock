package hx

import (
	"strings"
	"testing"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

func TestAttributes(t *testing.T) {
	cases := []struct {
		node g.Node
		want string
	}{
		{Get("/a"), `hx-get="/a"`},
		{Post("/a"), `hx-post="/a"`},
		{Put("/a"), `hx-put="/a"`},
		{Patch("/a"), `hx-patch="/a"`},
		{Delete("/a"), `hx-delete="/a"`},
		{Target("#m"), `hx-target="#m"`},
		{TargetInherited("#m"), `hx-target:inherited="#m"`},
		{Swap("outerHTML"), `hx-swap="outerHTML"`},
		{SwapInherited("none"), `hx-swap:inherited="none"`},
		{Select("#x"), `hx-select="#x"`},
		{Confirm(`Delete "x"?`), `hx-confirm="Delete &#34;x&#34;?"`},
		{Trigger("change"), `hx-trigger="change"`},
	}
	for _, tc := range cases {
		var b strings.Builder
		if err := h.Div(tc.node).Render(&b); err != nil {
			t.Fatal(err)
		}
		if want := "<div " + tc.want + "></div>"; b.String() != want {
			t.Errorf("got %s, want %s", b.String(), want)
		}
	}
}

func TestTriggerRefusesFilters(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("no panic for a trigger filter")
		}
	}()
	Trigger("keyup[key=='Enter']")
}
