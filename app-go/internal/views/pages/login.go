// Package pages renders the UI's full pages and the fragments htmx swaps into them. Pages
// are composed from components only and set no classes (spec §10.7).
package pages

import (
	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// LoginProps is what the login form shows.
type LoginProps struct {
	Email       string            // kept after a failed attempt; the password never is
	Error       string            // the message about the attempt, such as a wrong password
	FieldErrors map[string]string // "email" / "password" → message
}

// Login is the login page (spec §10.3 Login).
func Login(p LoginProps) g.Node {
	return layout.Bare(layout.BareProps{
		Title: "Login",
		Body: []g.Node{
			components.Brand(components.BrandProps{LogoSrc: layout.Asset("logo.svg"), Heading: true}),
			LoginForm(p),
		},
	})
}

// LoginForm is the login form, which htmx replaces with the server's answer. The button
// stays disabled until both fields are filled (Alpine component "requiredFields").
func LoginForm(p LoginProps) g.Node {
	return components.Form(components.FormProps{
		Action: "/login",
		Error:  p.Error,
		Attrs:  []g.Node{hx.Post("/login"), hx.Swap("outerHTML"), g.Attr("x-data", "requiredFields"), g.Attr("x-on:input", "update")},
		Fields: []g.Node{
			components.TextField(components.TextFieldProps{
				Label: "Email", Name: "email", Type: "email", Value: p.Email, Error: p.FieldErrors["email"],
				Required: true, Autofocus: p.Email == "", Autocomplete: "username",
			}),
			components.TextField(components.TextFieldProps{
				Label: "Password", Name: "password", Type: "password", Error: p.FieldErrors["password"],
				Required: true, Autofocus: p.Email != "", Autocomplete: "current-password",
			}),
		},
		Actions: []g.Node{
			components.Button(components.ButtonProps{
				Label: "Login", Variant: components.ButtonPrimary, Submit: true, Block: true, Large: true,
				Attrs: []g.Node{g.Attr("x-bind:disabled", "incomplete")},
			}),
		},
	})
}
