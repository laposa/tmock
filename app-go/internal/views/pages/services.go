package pages

import (
	"net/url"

	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// ServicesTableID is the id of the services table, which a successful save or delete
// replaces (HX-Retarget).
const ServicesTableID = "services-table"

// ServicesProps is what the services page shows.
type ServicesProps struct {
	Admin    bool
	Services []app.Service
}

// Services is the services page (spec §10.3 S1): the table, and "Add Service", which
// opens the add modal.
func Services(p ServicesProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Services",
		Current: layout.Services,
		Admin:   p.Admin,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{
				Title: "Services",
				Actions: []g.Node{components.Button(components.ButtonProps{
					Label: "Add Service", Variant: components.ButtonPrimary,
					Attrs: []g.Node{hx.Get("/ui/services/new"), hx.Target("#" + components.ModalBodyID)},
				})},
			}),
			ServicesTable(p.Services),
		},
	})
}

// ServicesTable is the table of services (S1): name (opens the edit modal) | upstream
// URL | path.
func ServicesTable(list []app.Service) g.Node {
	rows := make([]components.TableRow, 0, len(list))
	for _, s := range list {
		rows = append(rows, components.TableRow{Cells: []g.Node{
			components.LinkButton(components.LinkButtonProps{
				Label: s.Name,
				Attrs: []g.Node{hx.Get(ServiceURL(s.Path) + "/edit"), hx.Target("#" + components.ModalBodyID)},
			}),
			g.Text(s.UpstreamURL),
			g.Text(s.Path),
		}})
	}
	return components.Table(components.TableProps{
		Columns: []string{"Name", "Upstream URL", "Path"},
		Rows:    rows,
		Empty:   "No services yet. Add one to start proxying requests.",
		Attrs:   []g.Node{g.Attr("id", ServicesTableID)},
	})
}

// ServiceFormProps is what the add and edit modals show.
type ServiceFormProps struct {
	// Path is the path of the service being edited, as stored; empty in the add modal.
	// The form keeps posting to it while the path field is being changed.
	Path        string
	Values      app.ServiceInput
	Error       string            // a message about the whole form, such as a duplicate path
	FieldErrors map[string]string // "path" / "name" / "upstreamUrl" → message
}

// ServiceForm is the content of the add modal (S2) or, with Path set, the edit modal
// (S3). htmx swaps the answer to a failed save into the modal; a successful save replaces
// the table and closes the modal.
func ServiceForm(p ServiceFormProps) g.Node {
	edit := p.Path != ""
	title, action, save := "Add New Service", "/ui/services", "Create"
	if edit {
		title, action, save = "Edit Service", ServiceURL(p.Path), "Save"
	}
	var start []g.Node
	if edit {
		start = append(start, components.Button(components.ButtonProps{
			Label: "Delete", Variant: components.ButtonDanger,
			Attrs: []g.Node{
				hx.Delete(ServiceURL(p.Path)),
				hx.Confirm("Are you sure you want to delete the service " + p.Values.Name + "? Its scenarios will be deleted too."),
			},
		}))
	}
	field := func(label, name, typ, value string) g.Node {
		return components.TextField(components.TextFieldProps{
			Label: label, Name: name, ID: "service-" + name, Type: typ, Value: value,
			Error: p.FieldErrors[name], Required: true,
		})
	}
	return components.ModalContent(components.ModalContentProps{
		Title: title,
		Body: []g.Node{components.Form(components.FormProps{
			Action: action,
			Error:  p.Error,
			Attrs:  []g.Node{hx.Post(action), hx.Target("#" + components.ModalBodyID)},
			Fields: []g.Node{
				field("Name", "name", "text", p.Values.Name),
				field("Upstream URL", "upstreamUrl", "url", p.Values.UpstreamURL),
				components.TextField(components.TextFieldProps{
					Label: "Path", Name: "path", ID: "service-path", Value: p.Values.Path,
					Error: p.FieldErrors["path"], Required: true,
					Hint: "Requests to /api/proxy/<path>/… go to this service. Letters, digits, '.', '_' and '-'.",
				}),
			},
			Footer: components.ModalFooter(components.ModalFooterProps{
				Start: start,
				End: []g.Node{
					components.CloseModalButton(),
					components.Button(components.ButtonProps{Label: save, Variant: components.ButtonPrimary, Submit: true}),
				},
			}),
		})},
	})
}

// ServiceFormPage shows a service form as a page of its own, for a browser without
// JavaScript, which cannot open modals.
func ServiceFormPage(admin bool, p ServiceFormProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Services",
		Current: layout.Services,
		Admin:   admin,
		Body:    []g.Node{components.Card(components.CardProps{Body: []g.Node{ServiceForm(p)}})},
	})
}

// ServiceURL is the UI's URL of the service with path.
func ServiceURL(path string) string { return "/ui/services/" + url.PathEscape(path) }
