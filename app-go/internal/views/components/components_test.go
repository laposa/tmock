package components

import (
	"strings"
	"testing"

	g "maragu.dev/gomponents"
)

func render(t *testing.T, n g.Node) string {
	t.Helper()
	var b strings.Builder
	if err := n.Render(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

// check renders n and compares it with want exactly, so a test fails on any markup change
// that was not meant.
func check(t *testing.T, name string, n g.Node, want string) {
	t.Helper()
	if got := render(t, n); got != want {
		t.Errorf("%s\n got: %s\nwant: %s", name, got, want)
	}
}

var click = g.Attr("x-on:click", "go")

func TestButton(t *testing.T) {
	check(t, "default", Button(ButtonProps{Label: "Cancel"}),
		`<button type="button" class="button">Cancel</button>`)
	check(t, "primary submit, block, large", Button(ButtonProps{Label: "Login", Variant: ButtonPrimary, Submit: true, Block: true, Large: true}),
		`<button type="submit" class="button button--primary button--block button--large">Login</button>`)
	check(t, "danger with attrs", Button(ButtonProps{Label: "Delete", Variant: ButtonDanger, Attrs: []g.Node{click}}),
		`<button type="button" class="button button--danger" x-on:click="go">Delete</button>`)
	check(t, "text, disabled", Button(ButtonProps{Label: "<x>", Variant: ButtonText, Disabled: true}),
		`<button type="button" class="button button--text" disabled>&lt;x&gt;</button>`)
}

func TestTextField(t *testing.T) {
	check(t, "plain", TextField(TextFieldProps{Label: "Name", Name: "name"}),
		`<div class="field"><label class="field__label" for="name">Name</label>`+
			`<input class="field__input" id="name" name="name" type="text"></div>`)
	check(t, "all options", TextField(TextFieldProps{Label: "Email", Name: "email", ID: "login-email", Type: "email", Value: `a"b`,
		Required: true, Autofocus: true, Autocomplete: "username", Hint: "ignored when there is an error", Error: "is required",
		Attrs: []g.Node{click}}),
		`<div class="field"><label class="field__label" for="login-email">Email</label>`+
			`<input class="field__input" id="login-email" name="email" type="email" value="a&#34;b" required autofocus autocomplete="username" `+
			`aria-invalid="true" aria-describedby="login-email-message" x-on:click="go">`+
			`<p class="field__error" id="login-email-message">is required</p></div>`)
	check(t, "hint", TextField(TextFieldProps{Label: "Password", Name: "pw", Type: "password", Hint: "leave blank to keep"}),
		`<div class="field"><label class="field__label" for="pw">Password</label>`+
			`<input class="field__input" id="pw" name="pw" type="password" aria-describedby="pw-message">`+
			`<p class="field__hint" id="pw-message">leave blank to keep</p></div>`)
}

func TestForm(t *testing.T) {
	field := g.Text("[field]")
	action := g.Text("[action]")
	check(t, "error, fields, actions in that order",
		Form(FormProps{Action: "/login", Error: "Wrong", Fields: []g.Node{field}, Actions: []g.Node{action}, Attrs: []g.Node{click}}),
		`<form class="form" method="post" action="/login" x-on:click="go">`+
			`<div class="alert alert--error" role="alert">Wrong</div>[field]<div class="form__actions">[action]</div></form>`)
	check(t, "get, no error, no actions", Form(FormProps{Method: "get", Fields: []g.Node{field}}),
		`<form class="form" method="get">[field]</form>`)
	check(t, "footer instead of actions", Form(FormProps{Fields: []g.Node{field}, Actions: []g.Node{action}, Footer: g.Text("[footer]")}),
		`<form class="form" method="post">[field][footer]</form>`)
}

func TestAlert(t *testing.T) {
	check(t, "error", Alert(AlertProps{Message: "x"}), `<div class="alert alert--error" role="alert">x</div>`)
	check(t, "warning", Alert(AlertProps{Type: AlertWarning, Message: "x"}), `<div class="alert alert--warning" role="alert">x</div>`)
	check(t, "info", Alert(AlertProps{Type: AlertInfo, Message: "x"}), `<div class="alert alert--info" role="status">x</div>`)
	check(t, "success", Alert(AlertProps{Type: AlertSuccess, Message: "x", Attrs: []g.Node{click}}),
		`<div class="alert alert--success" role="status" x-on:click="go">x</div>`)
}

func TestBrand(t *testing.T) {
	check(t, "link", Brand(BrandProps{Href: "/", LogoSrc: "/l.svg"}),
		`<a class="brand brand--link" href="/"><img class="brand__logo" src="/l.svg" alt=""><span class="brand__name">Tmock</span></a>`)
	check(t, "heading", Brand(BrandProps{LogoSrc: "/l.svg", Heading: true}),
		`<div class="brand"><img class="brand__logo" src="/l.svg" alt=""><h1 class="brand__name">Tmock</h1></div>`)
}

func TestNav(t *testing.T) {
	check(t, "links then forms", Nav(NavProps{Label: "Main",
		Items: []NavItem{{Label: "A", Href: "/a"}, {Label: "B", Href: "/b", Current: true}},
		Forms: []NavForm{{Label: "Logout", Action: "/logout"}}}),
		`<nav class="nav" aria-label="Main"><a class="nav__link" href="/a">A</a>`+
			`<a class="nav__link" href="/b" aria-current="page">B</a>`+
			`<form class="nav__form" method="post" action="/logout"><button class="nav__link nav__link--button" type="submit">Logout</button></form></nav>`)
	check(t, "form with attrs", Nav(NavProps{Label: "Main", Forms: []NavForm{{Label: "Logout", Action: "/logout", Attrs: []g.Node{click}}}}),
		`<nav class="nav" aria-label="Main"><form class="nav__form" method="post" action="/logout" x-on:click="go">`+
			`<button class="nav__link nav__link--button" type="submit">Logout</button></form></nav>`)
}

func TestShell(t *testing.T) {
	check(t, "header: brand then nav", AppHeader(AppHeaderProps{Brand: g.Text("[brand]"), Nav: g.Text("[nav]")}),
		`<header class="app-header">[brand][nav]</header>`)
	check(t, "frame", Frame(g.Text("a"), g.Text("b")), `<div class="frame">ab</div>`)
	check(t, "main", Main(g.Text("a")), `<main class="main">a</main>`)
	check(t, "centered", Centered(g.Text("a")), `<main class="centered"><div class="centered__panel">a</div></main>`)
}

func TestPageHeader(t *testing.T) {
	check(t, "title only", PageHeader(PageHeaderProps{Title: "Services"}),
		`<div class="page-header"><h1 class="page-header__title">Services</h1></div>`)
	check(t, "title then actions", PageHeader(PageHeaderProps{Title: "Services", Actions: []g.Node{g.Text("[add]")}}),
		`<div class="page-header"><h1 class="page-header__title">Services</h1><div class="page-header__actions">[add]</div></div>`)
}

func TestCard(t *testing.T) {
	check(t, "with title", Card(CardProps{Title: "T", Body: []g.Node{g.Text("b")}, Attrs: []g.Node{click}}),
		`<section class="card" x-on:click="go"><h2 class="card__title">T</h2><div class="card__body">b</div></section>`)
	check(t, "without title", Card(CardProps{Body: []g.Node{g.Text("b")}}),
		`<section class="card"><div class="card__body">b</div></section>`)
}

func TestText(t *testing.T) {
	check(t, "plain", Text(TextProps{Children: []g.Node{g.Text("a")}}), `<p class="text">a</p>`)
	check(t, "muted", Text(TextProps{Muted: true, Children: []g.Node{g.Text("a")}, Attrs: []g.Node{click}}),
		`<p class="text text--muted" x-on:click="go">a</p>`)
}

func TestOutput(t *testing.T) {
	check(t, "output", Output(OutputProps{ID: "o", Attrs: []g.Node{click}}),
		`<output class="output" id="o" aria-live="polite" x-on:click="go"></output>`)
}

func TestToastRegion(t *testing.T) {
	check(t, "region", ToastRegion(),
		`<div class="toasts" id="toasts" x-data="toasts" aria-live="polite">`+
			`<template x-for="toast in items" x-bind:key="toast.id">`+
			`<div class="toast" x-bind:data-type="toast.type" role="status"><div class="toast__text">`+
			`<strong class="toast__title" x-text="toast.title"></strong>`+
			`<p class="toast__message" x-show="toast.message" x-text="toast.message"></p></div>`+
			`<button class="toast__close" type="button" aria-label="Dismiss" x-on:click="toast.dismiss">✕</button>`+
			`</div></template></div>`)
}

func TestModalHost(t *testing.T) {
	check(t, "host", ModalHost(),
		`<dialog class="modal" id="modal" x-data="modal" aria-labelledby="modal-title"><div class="modal__body" id="modal-body"></div></dialog>`)
}

func TestConfirmDialog(t *testing.T) {
	check(t, "confirm then cancel", ConfirmDialog(),
		`<dialog class="confirm" id="confirm" x-data="confirm" aria-describedby="confirm-message">`+
			`<p class="confirm__message" id="confirm-message" x-text="message"></p><div class="confirm__actions">`+
			`<button type="button" class="button button--danger" x-on:click="accept">Confirm</button>`+
			`<button type="button" class="button" x-on:click="cancel">Cancel</button></div></dialog>`)
}

func TestModalContent(t *testing.T) {
	check(t, "title then body", ModalContent(ModalContentProps{Title: "Edit <x>", Body: []g.Node{g.Text("[form]")}}),
		`<div class="modal-content"><h2 class="modal-content__title" id="modal-title">Edit &lt;x&gt;</h2>[form]</div>`)
	check(t, "persistent", ModalContent(ModalContentProps{Title: "T", Persistent: true}),
		`<div class="modal-content" data-persistent><h2 class="modal-content__title" id="modal-title">T</h2></div>`)
}

func TestModalFooter(t *testing.T) {
	check(t, "start then end", ModalFooter(ModalFooterProps{Start: []g.Node{g.Text("[delete]")}, End: []g.Node{g.Text("[close]"), g.Text("[save]")}}),
		`<div class="modal-footer"><div class="modal-footer__start">[delete]</div><div class="modal-footer__end">[close][save]</div></div>`)
	check(t, "empty start", ModalFooter(ModalFooterProps{End: []g.Node{g.Text("[close]")}}),
		`<div class="modal-footer"><div class="modal-footer__start"></div><div class="modal-footer__end">[close]</div></div>`)
	check(t, "close button", CloseModalButton(),
		`<button type="button" class="button" x-on:click="close">Close</button>`)
}

func TestTable(t *testing.T) {
	check(t, "rows", Table(TableProps{
		Columns: []string{"Name", "Path"},
		Rows: []TableRow{
			{Cells: []g.Node{g.Text("A"), g.Text("a")}},
			{Cells: []g.Node{g.Text("<B>"), g.Text("b")}, Attrs: []g.Node{click}},
		},
		Empty: "ignored when there are rows",
		Attrs: []g.Node{g.Attr("id", "t")},
	}),
		`<div class="table" id="t"><table class="table__table"><thead><tr>`+
			`<th class="table__head" scope="col">Name</th><th class="table__head" scope="col">Path</th></tr></thead><tbody>`+
			`<tr class="table__row"><td class="table__cell">A</td><td class="table__cell">a</td></tr>`+
			`<tr class="table__row" x-on:click="go"><td class="table__cell">&lt;B&gt;</td><td class="table__cell">b</td></tr>`+
			`</tbody></table></div>`)
	check(t, "empty", Table(TableProps{Columns: []string{"Name", "Path", "URL"}, Empty: "Nothing yet"}),
		`<div class="table"><table class="table__table"><thead><tr>`+
			`<th class="table__head" scope="col">Name</th><th class="table__head" scope="col">Path</th><th class="table__head" scope="col">URL</th></tr></thead><tbody>`+
			`<tr><td class="table__empty" colspan="3">Nothing yet</td></tr></tbody></table></div>`)
	check(t, "empty without message", Table(TableProps{Columns: []string{"Name"}}),
		`<div class="table"><table class="table__table"><thead><tr><th class="table__head" scope="col">Name</th></tr></thead><tbody></tbody></table></div>`)
}

func TestLinkButton(t *testing.T) {
	check(t, "link button", LinkButton(LinkButtonProps{Label: "My <svc>", Attrs: []g.Node{click}}),
		`<button type="button" class="link-button" x-on:click="go">My &lt;svc&gt;</button>`)
}

func TestSelect(t *testing.T) {
	check(t, "all options", Select(SelectProps{Label: "Service", Name: "service", ID: "s",
		Options: []SelectOption{{Value: "a", Label: "A <1>"}, {Value: "b"}}, Value: "b", Placeholder: "Pick",
		Required: true, Hint: "ignored when there is an error", Error: "bad", Attrs: []g.Node{click}}),
		`<div class="field"><label class="field__label" for="s">Service</label>`+
			`<select class="select" id="s" name="service" required aria-invalid="true" aria-describedby="s-message" x-on:click="go">`+
			`<option value="">Pick</option><option value="a">A &lt;1&gt;</option><option value="b" selected>b</option></select>`+
			`<p class="field__error" id="s-message">bad</p></div>`)
	check(t, "placeholder selected, hint", Select(SelectProps{Label: "M", Name: "m", Options: []SelectOption{{Value: "GET"}}, Placeholder: "Any", Hint: "h"}),
		`<div class="field"><label class="field__label" for="m">M</label><select class="select" id="m" name="m" aria-describedby="m-message">`+
			`<option value="" selected>Any</option><option value="GET">GET</option></select><p class="field__hint" id="m-message">h</p></div>`)
}

func TestSwitch(t *testing.T) {
	check(t, "checked, with error", Switch(SwitchProps{Label: "Skip", Name: "skip", ID: "sk", Checked: true, Error: "needs body", Attrs: []g.Node{click}}),
		`<div class="switch"><label class="switch__control"><input class="switch__input" type="checkbox" role="switch" id="sk" name="skip" checked `+
			`aria-invalid="true" aria-describedby="sk-message" x-on:click="go"><span class="switch__track" aria-hidden="true"></span>`+
			`<span class="switch__label">Skip</span></label><p class="switch__error" id="sk-message">needs body</p></div>`)
	check(t, "off, hidden label, value, disabled", Switch(SwitchProps{Label: "On", HideLabel: true, Value: "1", Disabled: true}),
		`<div class="switch"><label class="switch__control"><input class="switch__input" type="checkbox" role="switch" value="1" disabled>`+
			`<span class="switch__track" aria-hidden="true"></span><span class="switch__label switch__label--hidden">On</span></label></div>`)
}

func TestToggleField(t *testing.T) {
	check(t, "on: switch, label, control", ToggleField(ToggleFieldProps{ID: "t", Label: "Path", Name: "pathEnabled", On: true, Control: []g.Node{g.Text("[field]")}}),
		`<div class="toggle-field" id="t"><div class="toggle-field__switch"><div class="switch"><label class="switch__control">`+
			`<input class="switch__input" type="checkbox" role="switch" name="pathEnabled" checked><span class="switch__track" aria-hidden="true"></span>`+
			`<span class="switch__label switch__label--hidden">Path</span></label></div></div>`+
			`<span class="toggle-field__label" aria-hidden="true">Path</span><div class="toggle-field__control">[field]</div></div>`)
	check(t, "off", ToggleField(ToggleFieldProps{Label: "Code", Name: "codeEnabled", SwitchAttrs: []g.Node{click}}),
		`<div class="toggle-field"><div class="toggle-field__switch"><div class="switch"><label class="switch__control">`+
			`<input class="switch__input" type="checkbox" role="switch" name="codeEnabled" x-on:click="go"><span class="switch__track" aria-hidden="true"></span>`+
			`<span class="switch__label switch__label--hidden">Code</span></label></div></div>`+
			`<span class="toggle-field__label" aria-hidden="true">Code</span><div class="toggle-field__control"></div></div>`)
}

func TestFormSection(t *testing.T) {
	check(t, "title then fields", FormSection(FormSectionProps{Title: "Request <Conditions>", Fields: []g.Node{g.Text("[a]"), g.Text("[b]")}}),
		`<fieldset class="form-section"><legend class="form-section__title">Request &lt;Conditions&gt;</legend>[a][b]</fieldset>`)
}

func TestCollapsible(t *testing.T) {
	check(t, "open with meta", Collapsible(CollapsibleProps{Title: "Svc", Meta: "svc", Open: true, Body: []g.Node{g.Text("[t]")}, Attrs: []g.Node{click}}),
		`<details class="collapsible" open x-on:click="go"><summary class="collapsible__summary"><span class="collapsible__title">Svc</span>`+
			`<span class="collapsible__meta">svc</span></summary><div class="collapsible__body">[t]</div></details>`)
	check(t, "closed", Collapsible(CollapsibleProps{Title: "<Svc>"}),
		`<details class="collapsible"><summary class="collapsible__summary"><span class="collapsible__title">&lt;Svc&gt;</span></summary>`+
			`<div class="collapsible__body"></div></details>`)
}

func TestPopover(t *testing.T) {
	check(t, "preformatted", Popover(PopoverProps{ID: "p", Trigger: g.Text("[trigger]"), Text: "{\n  \"a\": 1\n}", Preformatted: true}),
		`<div class="popover" x-data="popover" x-on:mouseenter="show" x-on:mouseleave="hide" x-on:focusin="show" x-on:focusout="hide">[trigger]`+
			`<div class="popover__panel" id="p" popover="manual" role="tooltip"><pre class="popover__text popover__text--pre">{
  &#34;a&#34;: 1
}</pre></div></div>`)
	check(t, "text", Popover(PopoverProps{ID: "p", Trigger: g.Text("[trigger]"), Text: "<x>"}),
		`<div class="popover" x-data="popover" x-on:mouseenter="show" x-on:mouseleave="hide" x-on:focusin="show" x-on:focusout="hide">[trigger]`+
			`<div class="popover__panel" id="p" popover="manual" role="tooltip"><p class="popover__text">&lt;x&gt;</p></div></div>`)
}

func TestHeaderRows(t *testing.T) {
	row := func(name, value string) string {
		v := func(s string) string {
			if s == "" {
				return ""
			}
			return ` value="` + s + `"`
		}
		return `<div class="header-rows__row" data-header-row>` +
			`<input class="header-rows__input" name="hn" aria-label="Header name" placeholder="Header" autocomplete="off"` + v(name) + `>` +
			`<input class="header-rows__input" name="hv" aria-label="Header value" placeholder="Value (empty removes the header)" autocomplete="off"` + v(value) + `>` +
			`<button type="button" class="header-rows__remove" aria-label="Remove header" x-on:click="remove">✕</button></div>`
	}
	tail := `<template data-header-row-template>` + row("", "") + `</template>` +
		`<div class="header-rows__actions"><button type="button" class="button button--text" x-on:click="add">Add Header</button></div>`
	check(t, "rows and error", HeaderRows(HeaderRowsProps{Label: "Headers", NameField: "hn", ValueField: "hv",
		Rows: []HeaderRow{{Name: "A", Value: "1"}, {Name: "B"}}, Error: "dup"}),
		`<div class="header-rows" x-data="headerRows" role="group" aria-label="Headers"><span class="header-rows__label" aria-hidden="true">Headers</span><div class="header-rows__list" data-header-rows>`+
			row("A", "1")+row("B", "")+`</div>`+tail+`<p class="header-rows__error" role="alert">dup</p></div>`)
	check(t, "no rows: one empty row", HeaderRows(HeaderRowsProps{Label: "Headers", NameField: "hn", ValueField: "hv"}),
		`<div class="header-rows" x-data="headerRows" role="group" aria-label="Headers"><span class="header-rows__label" aria-hidden="true">Headers</span><div class="header-rows__list" data-header-rows>`+
			row("", "")+`</div>`+tail+`</div>`)
}

func TestCodeTextarea(t *testing.T) {
	check(t, "JSON with error; a leading newline survives", CodeTextarea(CodeTextareaProps{Label: "Body", Name: "body", ID: "b", Value: "\n{<a>}",
		Language: LanguageJSON, Error: "bad", Rows: 5, Attrs: []g.Node{click}}),
		`<div class="field"><label class="field__label" for="b">Body</label><div class="code-field" x-data="codeEditor" data-language="JSON">`+
			`<div class="code-field__toolbar"><span class="code-field__language" x-text="language">JSON</span>`+
			`<button type="button" class="button button--text" hidden x-bind:hidden="notJSON" x-on:click="format">Format JSON</button></div>`+
			`<textarea class="code-field__input" id="b" name="body" rows="5" spellcheck="false" wrap="off" autocapitalize="off" x-on:keydown.tab="indent" `+
			`aria-invalid="true" aria-describedby="b-message" x-on:click="go">`+"\n\n"+`{&lt;a&gt;}</textarea>`+
			`<p class="code-field__error" role="alert" x-show="formatError" x-text="formatError"></p></div>`+
			`<p class="field__error" id="b-message">bad</p></div>`)
	check(t, "defaults: plain text, 12 rows", CodeTextarea(CodeTextareaProps{Label: "Body", Name: "body"}),
		`<div class="field"><label class="field__label" for="body">Body</label><div class="code-field" x-data="codeEditor" data-language="Plain text">`+
			`<div class="code-field__toolbar"><span class="code-field__language" x-text="language">Plain text</span>`+
			`<button type="button" class="button button--text" hidden x-bind:hidden="notJSON" x-on:click="format">Format JSON</button></div>`+
			`<textarea class="code-field__input" id="body" name="body" rows="12" spellcheck="false" wrap="off" autocapitalize="off" x-on:keydown.tab="indent">`+"\n"+`</textarea>`+
			`<p class="code-field__error" role="alert" x-show="formatError" x-text="formatError"></p></div></div>`)
}

func TestChip(t *testing.T) {
	check(t, "strong, text and remove", Chip(ChipProps{Strong: "svc", Text: "slow <1>", RemoveLabel: "Remove svc | slow", RemoveAttrs: []g.Node{click}}),
		`<span class="chip"><span class="chip__text"><strong class="chip__strong">svc</strong> | slow &lt;1&gt;</span>`+
			`<button type="button" class="chip__remove" aria-label="Remove svc | slow" x-on:click="go">✕</button></span>`)
	check(t, "text only, not removable", Chip(ChipProps{Text: "plain"}),
		`<span class="chip"><span class="chip__text">plain</span></span>`)
}

func TestChipList(t *testing.T) {
	check(t, "chips then end", ChipList(ChipListProps{Chips: []g.Node{g.Text("[a]"), g.Text("[b]")}, End: g.Text("[add]")}),
		`<div class="chip-list">[a][b][add]</div>`)
	check(t, "no chips", ChipList(ChipListProps{End: g.Text("[add]")}), `<div class="chip-list">[add]</div>`)
}

func TestIconButton(t *testing.T) {
	check(t, "add", IconButton(IconButtonProps{Label: "Assign", Icon: IconAdd, Attrs: []g.Node{click}}),
		`<button type="button" class="icon-button" aria-label="Assign" title="Assign" x-on:click="go"><span aria-hidden="true">⊕</span></button>`)
	check(t, "info", IconButton(IconButtonProps{Label: "<i>", Icon: IconInfo}),
		`<button type="button" class="icon-button" aria-label="&lt;i&gt;" title="&lt;i&gt;"><span aria-hidden="true">ⓘ</span></button>`)
}

func TestButtonGroup(t *testing.T) {
	check(t, "in order", ButtonGroup(g.Text("[a]"), g.Text("[b]")), `<div class="button-group">[a][b]</div>`)
}

func TestCopyField(t *testing.T) {
	check(t, "value, copy button, warning", CopyField(CopyFieldProps{Label: "Token", ID: "tok", Value: "tmck_<x>", Copied: "Copied!", Warning: "once"}),
		`<div class="field"><label class="field__label" for="tok">Token</label><div class="copy-field" x-data="copyField" data-copied="Copied!">`+
			`<input class="copy-field__input" id="tok" type="text" readonly value="tmck_&lt;x&gt;" spellcheck="false" autocomplete="off" aria-describedby="tok-message" x-on:focus="select">`+
			`<button type="button" class="button button--primary" x-on:click="copy">Copy</button></div><p class="field__hint" id="tok-message">once</p></div>`)
}
