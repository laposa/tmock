// Package hx builds the htmx 4 attributes the views use (spec §10.1). It lives under
// internal/views, not internal/web, so that pages and components can use it without an
// import cycle (internal/web imports the views).
//
// Inheritance is explicit: htmx 4 attributes do not flow to children unless they carry
// the :inherited modifier, which the *Inherited helpers add. Request verbs and triggers
// are never inherited. Nothing here produces the htmx features that need eval (hx-on,
// js: values, trigger filters), so the strict CSP holds (spec §9.4).
package hx

import (
	"strings"

	g "maragu.dev/gomponents"
)

// Get issues a GET to url.
func Get(url string) g.Node { return g.Attr("hx-get", url) }

// Post issues a POST to url.
func Post(url string) g.Node { return g.Attr("hx-post", url) }

// Put issues a PUT to url.
func Put(url string) g.Node { return g.Attr("hx-put", url) }

// Patch issues a PATCH to url.
func Patch(url string) g.Node { return g.Attr("hx-patch", url) }

// Delete issues a DELETE to url.
func Delete(url string) g.Node { return g.Attr("hx-delete", url) }

// Target sets the element the response is swapped into (a CSS selector, or "this").
func Target(selector string) g.Node { return g.Attr("hx-target", selector) }

// TargetInherited is Target for every request inside the element.
func TargetInherited(selector string) g.Node { return g.Attr("hx-target:inherited", selector) }

// Swap sets how the response is swapped in, for example "outerHTML" or "none".
func Swap(style string) g.Node { return g.Attr("hx-swap", style) }

// SwapInherited is Swap for every request inside the element.
func SwapInherited(style string) g.Node { return g.Attr("hx-swap:inherited", style) }

// Select picks the part of the response to swap in.
func Select(selector string) g.Node { return g.Attr("hx-select", selector) }

// Sync coordinates the element's requests with others, for example "closest form:drop":
// while a request of the form is in flight, a new one is dropped.
func Sync(spec string) g.Node { return g.Attr("hx-sync", spec) }

// Confirm asks before the request is sent. app.js shows the question in the confirmation
// dialog (spec §10.2) instead of window.confirm.
func Confirm(question string) g.Node { return g.Attr("hx-confirm", question) }

// Trigger sets the events that issue the request, for example "change" or
// "input changed delay:300ms". Filters in brackets need eval, so they are refused.
func Trigger(spec string) g.Node {
	if strings.Contains(spec, "[") {
		panic("hx.Trigger: trigger filters need eval and break the CSP: " + spec)
	}
	return g.Attr("hx-trigger", spec)
}
