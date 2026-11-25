// Package components holds every UI element as its own component (spec §10.7): one Go
// function per element, in its own file, with a section of the same name in
// static/app.css.
//
// Rules:
//
//   - A component takes a …Props struct with what it shows and what state it is in, and a
//     typed variant where there is a choice. Callers never pass class names or styles.
//     Behaviour attributes (id, hx-*, x-*, aria-*) go in Attrs and land on the component's
//     main element.
//   - This is the only package that sets class attributes. Class names are BEM-style and
//     named after the component (button, button--danger, field__error). A test fails if
//     another UI package sets one.
//   - Components that hold others (AppHeader, Form, ConfirmDialog, …) decide the order of
//     their parts, so that a layout change is one edit here, not one per page. The order
//     is the DOM order, never CSS order, so keyboard and screen-reader order match what
//     users see.
package components

import (
	g "maragu.dev/gomponents"
)

// attrs groups a component's caller-supplied attributes.
func attrs(a []g.Node) g.Node { return g.Group(a) }
