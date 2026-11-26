package pages

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// ScenariosListID is the id of the grouped scenario list, which a successful save or
// delete replaces (HX-Retarget).
const ScenariosListID = "scenarios-list"

// Ids in the editor that "Show Headers" and "Show Body" scroll to (spec §10.3 SC1).
const (
	ScenarioHeadersID = "scenario-headers"
	ScenarioBodyID    = "scenario-body"
)

// Focus values of the editor: the part it opens scrolled to.
const (
	FocusHeaders = "headers"
	FocusBody    = "body"
)

// bodyPreviewLength is how much of a body the list's popover shows (SC1).
const bodyPreviewLength = 255

// UnsavedQuestion is what the editor asks before it closes with unsaved changes (SC4).
// app.js has the same text.
const UnsavedQuestion = "You will lose unsaved changes. Are you sure you want to close?"

// ScenariosProps is what the scenarios page shows.
type ScenariosProps struct {
	Admin     bool
	Scenarios []app.Scenario // ordered by service path, then name
	Services  []app.Service  // for the names of the groups
}

// Scenarios is the scenarios page (spec §10.3 SC1, SC2): the scenarios grouped by
// service, and "Add Scenario", which opens the editor.
func Scenarios(p ScenariosProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Scenarios",
		Current: layout.Scenarios,
		Admin:   p.Admin,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{
				Title: "Scenarios",
				Actions: []g.Node{components.Button(components.ButtonProps{
					Label: "Add Scenario", Variant: components.ButtonPrimary,
					Attrs: []g.Node{hx.Get("/ui/scenarios/new"), hx.Target("#" + components.ModalBodyID)},
				})},
			}),
			ScenariosList(p.Scenarios, p.Services),
		},
	})
}

var scenarioColumns = []string{"Name", "Method", "Path", "Condition", "Code", "Headers", "Body"}

// ScenariosList is the scenarios in one collapsible group per service (SC1), in the
// order given. Groups are open, so that they stay open when the list is replaced after
// a save.
func ScenariosList(scenarios []app.Scenario, services []app.Service) g.Node {
	if len(scenarios) == 0 {
		return h.Div(h.ID(ScenariosListID), components.Table(components.TableProps{
			Columns: scenarioColumns,
			Empty:   "No scenarios yet. Add one to mock a response.",
		}))
	}
	names := map[string]string{}
	for _, s := range services {
		names[s.Path] = s.Name
	}
	var groups []g.Node
	for start := 0; start < len(scenarios); {
		path := scenarios[start].Service
		end := start
		for end < len(scenarios) && scenarios[end].Service == path {
			end++
		}
		title := names[path]
		if title == "" {
			title = path
		}
		rows := make([]components.TableRow, 0, end-start)
		for _, s := range scenarios[start:end] {
			rows = append(rows, scenarioRow(s))
		}
		groups = append(groups, components.Collapsible(components.CollapsibleProps{
			Title: title, Meta: path, Open: true,
			Body: []g.Node{components.Table(components.TableProps{Columns: scenarioColumns, Rows: rows})},
		}))
		start = end
	}
	return h.Div(h.ID(ScenariosListID), g.Group(groups))
}

// scenarioRow is one row of the list: Name (opens the editor) | Method | Path |
// Condition | Code | Headers | Body. "Show Headers" and "Show Body" preview the value
// in a popover and open the editor scrolled to it.
func scenarioRow(s app.Scenario) components.TableRow {
	open := func(label, focus string, more ...g.Node) g.Node {
		url := ScenarioURL(s.ID) + "/edit"
		if focus != "" {
			url += "?focus=" + focus
		}
		return components.LinkButton(components.LinkButtonProps{
			Label: label,
			Attrs: append([]g.Node{hx.Get(url), hx.Target("#" + components.ModalBodyID)}, more...),
		})
	}
	preview := func(label, focus, text string) g.Node {
		id := fmt.Sprintf("scenario-%d-%s", s.ID, focus)
		return components.Popover(components.PopoverProps{
			ID: id, Text: text, Preformatted: true,
			Trigger: open(label, focus, g.Attr("aria-describedby", id)),
		})
	}
	var headers, body g.Node
	if s.ResponseHeaders != nil {
		headers = preview("Show Headers", FocusHeaders, headersJSON(s.ResponseHeaders))
	}
	if s.ResponseBody != nil {
		body = preview("Show Body", FocusBody, truncate(*s.ResponseBody, bodyPreviewLength))
	}
	code := ""
	if s.ResponseCode != nil {
		code = strconv.Itoa(*s.ResponseCode)
	}
	return components.TableRow{Cells: []g.Node{
		open(s.Name, ""),
		g.Text(deref(s.RequestMethod)),
		g.Text(deref(s.RequestPath)),
		g.Text(deref(s.RequestCondition)),
		g.Text(code),
		headers,
		body,
	}}
}

// HeaderRow is a response header as the editor shows it.
type HeaderRow = components.HeaderRow

// ScenarioFormValues are the editor's fields as the user sees them: each optional field
// has its switch and its text, which is kept while the switch is off.
type ScenarioFormValues struct {
	Name        string
	Service     string // the service's path
	MethodOn    bool
	Method      string
	PathOn      bool
	Path        string
	ConditionOn bool
	Condition   string
	CodeOn      bool
	Code        string
	HeadersOn   bool
	Headers     []HeaderRow // an empty value is a header the response drops (null)
	BodyOn      bool
	Body        string
	SkipProxy   bool
}

// ScenarioValues are the editor's fields for a stored scenario: a switch is on exactly
// when the field is set. Headers are in name order.
func ScenarioValues(s app.Scenario) ScenarioFormValues {
	v := ScenarioFormValues{
		Name: s.Name, Service: s.Service,
		MethodOn: s.RequestMethod != nil, Method: deref(s.RequestMethod),
		PathOn: s.RequestPath != nil, Path: deref(s.RequestPath),
		ConditionOn: s.RequestCondition != nil, Condition: deref(s.RequestCondition),
		CodeOn:    s.ResponseCode != nil,
		HeadersOn: s.ResponseHeaders != nil,
		BodyOn:    s.ResponseBody != nil, Body: deref(s.ResponseBody),
		SkipProxy: s.SkipProxy,
	}
	if s.ResponseCode != nil {
		v.Code = strconv.Itoa(*s.ResponseCode)
	}
	names := make([]string, 0, len(s.ResponseHeaders))
	for n := range s.ResponseHeaders {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		v.Headers = append(v.Headers, HeaderRow{Name: n, Value: deref(s.ResponseHeaders[n])})
	}
	return v
}

// ScenarioFormProps is what the editor shows.
type ScenarioFormProps struct {
	ID          int64 // the scenario being edited; 0 in the add modal
	Values      ScenarioFormValues
	Services    []app.Service // the choices of the Service select
	Focus       string        // FocusHeaders or FocusBody: the editor opens scrolled there
	Unsaved     bool          // the values are not what is stored (a failed save)
	Error       string        // a message about the whole form, such as a duplicate name
	FieldErrors map[string]string
}

// ScenarioForm is the scenario editor (spec §10.3 SC2–SC5, §10.4), loaded into the modal.
// It does not close on a click on the backdrop, and the Alpine component
// "scenarioEditor" in app.js asks before it closes with unsaved changes.
func ScenarioForm(p ScenarioFormProps) g.Node {
	edit := p.ID != 0
	title, action, save := "Add Scenario", "/ui/scenarios", "Create"
	if edit {
		title, action, save = "Edit Scenario", ScenarioURL(p.ID), "Save"
	}
	v, errs := p.Values, p.FieldErrors

	var start []g.Node
	if edit {
		start = append(start, components.Button(components.ButtonProps{
			Label: "Delete", Variant: components.ButtonDanger,
			Attrs: []g.Node{
				hx.Delete(ScenarioURL(p.ID)),
				hx.Confirm("Are you sure you want to delete the scenario " + v.Name + "?"),
			},
		}))
	}
	services := make([]components.SelectOption, 0, len(p.Services))
	for _, s := range p.Services {
		services = append(services, components.SelectOption{Value: s.Path, Label: s.Name})
	}
	methods := make([]components.SelectOption, 0, len(app.RequestMethods))
	for _, m := range app.RequestMethods {
		methods = append(methods, components.SelectOption{Value: m})
	}
	formAttrs := []g.Node{hx.Post(action), hx.Target("#" + components.ModalBodyID),
		g.Attr("x-data", "scenarioEditor"), g.Attr("x-on:input", "update"), g.Attr("x-on:change", "changed")}
	if p.Unsaved {
		formAttrs = append(formAttrs, g.Attr("data-unsaved"))
	}
	switch p.Focus {
	case FocusHeaders:
		formAttrs = append(formAttrs, g.Attr("data-scroll-to", ScenarioHeadersID))
	case FocusBody:
		formAttrs = append(formAttrs, g.Attr("data-scroll-to", ScenarioBodyID))
	}

	toggle := func(id, label, name string, on bool, control ...g.Node) g.Node {
		return components.ToggleField(components.ToggleFieldProps{ID: id, Label: label, Name: name + "Enabled", On: on, Control: control})
	}
	text := func(label, name, value, hint string) g.Node {
		return components.TextField(components.TextFieldProps{
			Label: label, Name: name, ID: "scenario-" + name, Value: value, Error: errs[name], Hint: hint,
		})
	}

	return components.ModalContent(components.ModalContentProps{
		Title:      title,
		Persistent: true,
		Body: []g.Node{components.Form(components.FormProps{
			Action: action,
			Error:  p.Error,
			Attrs:  formAttrs,
			Fields: []g.Node{
				components.TextField(components.TextFieldProps{
					Label: "Name", Name: "name", ID: "scenario-name", Value: v.Name, Error: errs["name"], Required: true,
				}),
				components.Select(components.SelectProps{
					Label: "Service", Name: "service", ID: "scenario-service", Options: services, Value: v.Service,
					Placeholder: "Select a service", Error: errs["service"], Required: true,
					Hint: noServicesHint(len(p.Services)),
				}),
				components.FormSection(components.FormSectionProps{Title: "Request Conditions", Fields: []g.Node{
					toggle("", "Request Method", "requestMethod", v.MethodOn, components.Select(components.SelectProps{
						Label: "Request Method", Name: "requestMethod", ID: "scenario-requestMethod",
						Options: methods, Value: v.Method, Placeholder: "Any method", Error: errs["requestMethod"],
					})),
					toggle("", "Request Path", "requestPath", v.PathOn, text("Request Path", "requestPath", v.Path,
						"The path after /api/proxy/<service>/, without the query string; compared case-insensitively.")),
					toggle("", "Request Condition", "requestCondition", v.ConditionOn, text("Request Condition", "requestCondition", v.Condition,
						"For example: $request.method == 'POST' && $request.query.page == '2'")),
				}}),
				components.FormSection(components.FormSectionProps{Title: "Response Modifications", Fields: []g.Node{
					toggle("", "Response Code", "responseCode", v.CodeOn, components.TextField(components.TextFieldProps{
						Label: "Response Code", Name: "responseCode", ID: "scenario-responseCode", Type: "number",
						Value: v.Code, Error: errs["responseCode"],
					})),
					toggle(ScenarioHeadersID, "Response Headers", "responseHeaders", v.HeadersOn, components.HeaderRows(components.HeaderRowsProps{
						Label: "Response Headers", NameField: "headerName", ValueField: "headerValue",
						Rows: v.Headers, Error: errs["responseHeaders"],
					})),
					toggle(ScenarioBodyID, "Response Body", "responseBody", v.BodyOn,
						components.CodeTextarea(components.CodeTextareaProps{
							Label: "Response Body", Name: "responseBody", ID: "scenario-responseBody", Value: v.Body,
							Language: BodyLanguage(v.Headers), Error: errs["responseBody"],
						}),
						components.Switch(components.SwitchProps{
							Label: "Skip Proxy (answer without calling the upstream)", Name: "skipProxy", ID: "scenario-skipProxy",
							Checked: v.SkipProxy, Error: errs["skipProxy"],
						}),
					),
				}}),
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

// ScenarioFormPage shows the editor as a page of its own, for a browser without
// JavaScript, which cannot open modals.
func ScenarioFormPage(admin bool, p ScenarioFormProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Scenarios",
		Current: layout.Scenarios,
		Admin:   admin,
		Body:    []g.Node{components.Card(components.CardProps{Body: []g.Node{ScenarioForm(p)}})},
	})
}

// BodyLanguage is the language hint of the body editor, from the Content-Type header
// row (as the legacy editor did): HTML, JSON or plain text. app.js has the same rule, so
// the hint follows the header rows while they are edited.
func BodyLanguage(headers []HeaderRow) string {
	for _, r := range headers {
		if !strings.EqualFold(strings.TrimSpace(r.Name), "Content-Type") {
			continue
		}
		v := strings.ToLower(r.Value)
		switch {
		case strings.Contains(v, "html"):
			return components.LanguageHTML
		case strings.Contains(v, "json"):
			return components.LanguageJSON
		}
		return components.LanguagePlain
	}
	return components.LanguagePlain
}

// ScenarioURL is the UI's URL of the scenario with id.
func ScenarioURL(id int64) string { return "/ui/scenarios/" + strconv.FormatInt(id, 10) }

func noServicesHint(n int) string {
	if n == 0 {
		return "There are no services yet. Add one on the Services page first."
	}
	return ""
}

// headersJSON is the headers as indented JSON, for the list's popover.
func headersJSON(h map[string]*string) string {
	b, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return ""
	}
	return string(b)
}

// truncate shortens s to n characters, marking the cut with "…".
func truncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "…"
}

func deref[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}
