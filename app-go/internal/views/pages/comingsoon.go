// Package pages renders the UI's full pages.
package pages

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// ComingSoon is the placeholder shown at / until the dashboard pages exist (spec §2.2).
func ComingSoon() g.Node {
	return layout.Page(layout.Props{
		Title: "Coming soon",
		Body: []g.Node{
			h.Section(h.Class("card"),
				h.H1(g.Text("Tmock — coming soon")),
				h.P(g.Text("The Go version of Tmock is being built iteration by iteration. "),
					g.Text("The dashboard will appear here."),
				),
				// Alpine replaces this text when it runs (component in static/app.js).
				h.P(h.Class("muted"), g.Attr("x-data", "jsStatus"),
					h.Span(g.Attr("x-text", "message"), g.Text("JavaScript is not running.")),
				),
				// htmx loads the health check into the output element.
				h.P(
					h.Button(h.Class("button"), h.Type("button"),
						g.Attr("hx-get", "/api/status"),
						g.Attr("hx-target", "#api-status"),
						g.Text("Check API status"),
					),
					h.Output(h.ID("api-status"), h.Class("output"), h.Aria("live", "polite")),
				),
			),
		},
	})
}
