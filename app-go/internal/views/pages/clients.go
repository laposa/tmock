package pages

import (
	"fmt"
	"strconv"
	"strings"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// ClientsTableID is the id of the clients table, which saves replace (HX-Retarget).
const ClientsTableID = "clients-table"

// ClientsChangedEvent is raised (HX-Trigger) when a change made in a modal should show in
// the clients table: the table reloads itself.
const ClientsChangedEvent = "clients:changed"

// ClientsProps is what the clients page shows.
type ClientsProps struct {
	Admin   bool
	Clients []app.Client
}

// Clients is the clients page (spec §10.3 C1–C8) at /.
func Clients(p ClientsProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Clients",
		Current: layout.Clients,
		Admin:   p.Admin,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{
				Title: "Clients",
				Actions: []g.Node{components.Button(components.ButtonProps{
					Label: "Add Client", Variant: components.ButtonPrimary,
					Attrs: []g.Node{hx.Get("/ui/clients/new"), hx.Target("#" + components.ModalBodyID)},
				})},
			}),
			ClientsTable(p.Clients),
		},
	})
}

// ClientsTable is the table of clients (C1): enabled switch | name (opens the edit
// modal) | "Conditions" | scenario chips with ✕ and ⊕ | token actions. It reloads itself
// on ClientsChangedEvent, so the modals need not know it.
func ClientsTable(list []app.Client) g.Node {
	modal := hx.Target("#" + components.ModalBodyID)
	rows := make([]components.TableRow, 0, len(list))
	for _, c := range list {
		url := ClientURL(c.ID)
		chips := make([]g.Node, 0, len(c.Assigned))
		for _, s := range c.Assigned {
			chips = append(chips, components.Chip(components.ChipProps{
				Strong: s.Service, Text: s.Name,
				RemoveLabel: fmt.Sprintf("Remove %s | %s", s.Service, s.Name),
				RemoveAttrs: []g.Node{hx.Delete(fmt.Sprintf("%s/scenarios/%d", url, s.ID))},
			}))
		}
		token := components.Button(components.ButtonProps{Label: "Generate", Variant: components.ButtonText,
			Attrs: []g.Node{hx.Post(url + "/token"), modal}})
		if c.HasToken {
			token = components.ButtonGroup(
				components.Button(components.ButtonProps{Label: "Regenerate", Variant: components.ButtonText,
					Attrs: []g.Node{hx.Post(url + "/token"), modal,
						hx.Confirm("Regenerate the token of " + c.Name + "? The current token stops working.")}}),
				components.Button(components.ButtonProps{Label: "Revoke", Variant: components.ButtonText,
					Attrs: []g.Node{hx.Delete(url + "/token"),
						hx.Confirm("Are you sure you want to revoke the token of the client " + c.Name + "?")}}),
			)
		}
		rows = append(rows, components.TableRow{Cells: []g.Node{
			components.Switch(components.SwitchProps{
				Label: "Enabled: " + c.Name, HideLabel: true, Name: "enabled", Checked: c.Enabled,
				Attrs: []g.Node{hx.Post(url + "/enabled"), hx.Trigger("change")},
			}),
			components.LinkButton(components.LinkButtonProps{Label: c.Name, Attrs: []g.Node{hx.Get(url + "/edit"), modal}}),
			components.LinkButton(components.LinkButtonProps{Label: "Conditions",
				Attrs: []g.Node{hx.Get(ConditionURL(c.ID)), modal}}),
			components.ChipList(components.ChipListProps{Chips: chips, End: components.IconButton(components.IconButtonProps{
				Label: "Assign scenarios to " + c.Name, Icon: components.IconAdd,
				Attrs: []g.Node{hx.Get(url + "/scenarios"), modal},
			})}),
			token,
		}})
	}
	return components.Table(components.TableProps{
		Columns: []string{"Enabled", "Name", "Conditions", "Scenarios", "Token"},
		Rows:    rows,
		Empty:   "No clients yet. Add one, give it a condition and enable it.",
		Attrs: []g.Node{g.Attr("id", ClientsTableID),
			hx.Get("/ui/clients/table"), hx.Trigger(ClientsChangedEvent + " from:body"), hx.Target("this"), hx.Swap("outerHTML")},
	})
}

// ClientFormProps is what the add and edit modals show.
type ClientFormProps struct {
	ID   int64 // 0 in the add modal
	Name string
	// StoredName is the client's name as stored, for the Delete confirmation, when Name is
	// what was typed (a failed save); empty means Name.
	StoredName  string
	Error       string
	FieldErrors map[string]string // "name"
}

// ClientForm is the add modal (C3: name only; the client starts disabled) or, with ID,
// the edit modal (C4: rename, Delete). The condition has its own modal, the builder.
func ClientForm(p ClientFormProps) g.Node {
	edit := p.ID != 0
	title, action, save := "Add Client", "/ui/clients", "Create"
	if edit {
		title, action, save = "Edit Client", ClientURL(p.ID), "Save"
	}
	var start []g.Node
	if edit {
		stored := p.StoredName
		if stored == "" {
			stored = p.Name
		}
		start = append(start, components.Button(components.ButtonProps{
			Label: "Delete", Variant: components.ButtonDanger,
			Attrs: []g.Node{hx.Delete(ClientURL(p.ID)), hx.Confirm("Are you sure you want to delete the client " + stored + "?")},
		}))
	}
	return components.ModalContent(components.ModalContentProps{
		Title: title,
		Body: []g.Node{components.Form(components.FormProps{
			Action: action,
			Error:  p.Error,
			Attrs:  []g.Node{hx.Post(action), hx.Target("#" + components.ModalBodyID)},
			Fields: []g.Node{components.TextField(components.TextFieldProps{
				Label: "Name", Name: "name", ID: "client-name", Value: p.Name, Error: p.FieldErrors["name"], Required: true,
			})},
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

// ClientFormPage shows a client form as a page of its own, for a browser without
// JavaScript.
func ClientFormPage(admin bool, p ClientFormProps) g.Node {
	return clientPage(admin, ClientForm(p))
}

// AssignmentsProps is what the assignment modal shows.
type AssignmentsProps struct {
	Client    app.Client
	Scenarios []app.Scenario // ordered by service path, then name
	Services  []app.Service
}

// Assignments is the assignment modal (C6): one collapsible group per service; each
// scenario has a switch that saves at once, and an info icon with its settings.
func Assignments(p AssignmentsProps) g.Node {
	names := map[string]string{}
	for _, s := range p.Services {
		names[s.Path] = s.Name
	}
	assigned := map[int64]bool{}
	for _, id := range p.Client.Scenarios {
		assigned[id] = true
	}
	var groups []g.Node
	for start := 0; start < len(p.Scenarios); {
		path := p.Scenarios[start].Service
		end := start
		for end < len(p.Scenarios) && p.Scenarios[end].Service == path {
			end++
		}
		var rows []components.TableRow
		n := 0
		for _, s := range p.Scenarios[start:end] {
			if assigned[s.ID] {
				n++
			}
			info := fmt.Sprintf("scenario-%d-info", s.ID)
			rows = append(rows, components.TableRow{Cells: []g.Node{
				AssignmentSwitch(p.Client.ID, s, assigned[s.ID]),
				components.Popover(components.PopoverProps{
					ID: info, Text: ScenarioSummary(s), Preformatted: true,
					Trigger: components.IconButton(components.IconButtonProps{Label: "Settings of " + s.Name, Icon: components.IconInfo,
						Attrs: []g.Node{h.Aria("describedby", info)}}),
				}),
			}})
		}
		title := names[path]
		if title == "" {
			title = path
		}
		groups = append(groups, components.Collapsible(components.CollapsibleProps{
			Title: title, Meta: path, Open: n > 0 || len(p.Scenarios) == end-start,
			Body: []g.Node{components.Table(components.TableProps{Columns: []string{"Assigned", "Settings"}, Rows: rows,
				Widths: []components.ColumnWidth{components.ColumnFill, components.ColumnSmall}})},
		}))
		start = end
	}
	if len(groups) == 0 {
		groups = append(groups, components.Text(components.TextProps{Muted: true,
			Children: []g.Node{g.Text("There are no scenarios yet. Add them on the Scenarios page.")}}))
	}
	return components.ModalContent(components.ModalContentProps{
		Title: "Scenarios of " + p.Client.Name,
		Body: append(groups, components.ModalFooter(components.ModalFooterProps{
			End: []g.Node{components.CloseModalButton()},
		})),
	})
}

// AssignmentSwitch is one scenario's switch in the assignment modal. Toggling it posts
// at once; the answer replaces it with the switch as saved, so a failed save reverts it.
func AssignmentSwitch(clientID int64, s app.Scenario, assigned bool) g.Node {
	id := fmt.Sprintf("assignment-%d-%d", clientID, s.ID)
	return h.Div(h.ID(id), components.Switch(components.SwitchProps{
		Label: s.Name, Name: "assigned", Checked: assigned,
		Attrs: []g.Node{hx.Post(fmt.Sprintf("%s/scenarios/%d", ClientURL(clientID), s.ID)), hx.Trigger("change"),
			hx.Target("#" + id), hx.Swap("outerHTML")},
	}))
}

// ScenarioSummary is a scenario's settings as lines of text, for the assignment modal's
// info popover.
func ScenarioSummary(s app.Scenario) string {
	var b strings.Builder
	line := func(label, value string) { fmt.Fprintf(&b, "%-10s %s\n", label+":", value) }
	orAny := func(p *string) string {
		if p == nil {
			return "any"
		}
		return *p
	}
	line("Method", orAny(s.RequestMethod))
	line("Path", orAny(s.RequestPath))
	if s.RequestCondition != nil {
		line("Condition", *s.RequestCondition)
	}
	if s.ResponseCode != nil {
		line("Code", strconv.Itoa(*s.ResponseCode))
	}
	if s.ResponseHeaders != nil {
		line("Headers", headersJSON(s.ResponseHeaders))
	}
	if s.ResponseBody != nil {
		line("Body", truncate(*s.ResponseBody, bodyPreviewLength))
	}
	if s.SkipProxy {
		line("Proxy", "skipped")
	}
	return strings.TrimRight(b.String(), "\n")
}

// TokenModal shows a token right after it was generated (C8): it is shown only once.
func TokenModal(clientName, token string) g.Node {
	return components.ModalContent(components.ModalContentProps{
		Title:      "Access Token of " + clientName,
		Persistent: true, // a stray click must not lose the token
		Body: []g.Node{
			components.Alert(components.AlertProps{Type: components.AlertWarning,
				Message: "This token is shown only once. Copy it now; it cannot be shown again."}),
			components.CopyField(components.CopyFieldProps{Label: "Token", ID: "client-token", Value: token,
				Copied:  "Token copied to clipboard",
				Warning: `Send it as the X-Client-Token header to GET and PATCH /api/client/<id> (enabled and scenarios only).`}),
			components.ModalFooter(components.ModalFooterProps{End: []g.Node{components.CloseModalButton()}}),
		},
	})
}

// TokenPage shows the token as a page of its own, for a browser without JavaScript.
func TokenPage(admin bool, clientName, token string) g.Node {
	return clientPage(admin, TokenModal(clientName, token))
}

// AssignmentsPage shows the assignment modal as a page of its own (without JavaScript the
// switches cannot save; this page at least shows the assignments).
func AssignmentsPage(admin bool, p AssignmentsProps) g.Node {
	return clientPage(admin, Assignments(p))
}

func clientPage(admin bool, content g.Node) g.Node {
	return layout.Page(layout.Props{
		Title:   "Clients",
		Current: layout.Clients,
		Admin:   admin,
		Body:    []g.Node{components.Card(components.CardProps{Body: []g.Node{content}})},
	})
}

// ClientURL is the UI's URL of the client with id.
func ClientURL(id int64) string { return "/ui/clients/" + strconv.FormatInt(id, 10) }
