package pages

import (
	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// PlaceholderProps configures a Placeholder.
type PlaceholderProps struct {
	Section   layout.Section
	Title     string
	Iteration string // the iteration that builds the page, for example "I05"
	Admin     bool
}

// Placeholder stands in for a page of the dashboard until its iteration builds it. It
// also shows that htmx and Alpine run (spec §15 I00).
func Placeholder(p PlaceholderProps) g.Node {
	return layout.Page(layout.Props{
		Title:   p.Title,
		Current: p.Section,
		Admin:   p.Admin,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{Title: p.Title}),
			components.Card(components.CardProps{
				Title: "Coming soon",
				Body: []g.Node{
					components.Text(components.TextProps{Children: []g.Node{
						g.Textf("This page is built in iteration %s of the Go rewrite.", p.Iteration),
					}}),
					// Alpine replaces this text when it runs (component in static/app.js).
					components.Text(components.TextProps{Muted: true, Attrs: []g.Node{g.Attr("x-data", "jsStatus"), g.Attr("x-text", "message")},
						Children: []g.Node{g.Text("JavaScript is not running.")}}),
					// htmx loads the health check into the output element.
					components.Text(components.TextProps{Children: []g.Node{
						components.Button(components.ButtonProps{Label: "Check API status",
							Attrs: []g.Node{hx.Get("/api/status"), hx.Target("#api-status")}}),
						components.Output(components.OutputProps{ID: "api-status"}),
					}}),
				},
			}),
		},
	})
}
