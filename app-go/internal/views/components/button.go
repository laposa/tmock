package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ButtonVariant is the look of a button, after the colours the legacy dashboard used.
type ButtonVariant int

const (
	ButtonDefault ButtonVariant = iota // neutral surface (legacy "Cancel", "Close")
	ButtonPrimary                      // indigo (legacy color="indigo")
	ButtonDanger                       // red (legacy color="red": delete, confirm)
	ButtonText                         // no background (legacy variant="text")
)

// ButtonProps configures a Button.
type ButtonProps struct {
	Label    string
	Variant  ButtonVariant
	Submit   bool // type="submit"; otherwise type="button"
	Block    bool // full width
	Large    bool
	Disabled bool
	Attrs    []g.Node // id, name, value, hx-*, x-* …
}

// Button is a <button>.
func Button(p ButtonProps) g.Node {
	typ := "button"
	if p.Submit {
		typ = "submit"
	}
	return h.Button(
		h.Type(typ),
		h.Class(buttonClass(p)),
		g.If(p.Disabled, h.Disabled()),
		attrs(p.Attrs),
		g.Text(p.Label),
	)
}

func buttonClass(p ButtonProps) string {
	c := "button"
	switch p.Variant {
	case ButtonPrimary:
		c += " button--primary"
	case ButtonDanger:
		c += " button--danger"
	case ButtonText:
		c += " button--text"
	}
	if p.Block {
		c += " button--block"
	}
	if p.Large {
		c += " button--large"
	}
	return c
}
