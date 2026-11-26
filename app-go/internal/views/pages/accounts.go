package pages

import (
	"strconv"

	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
)

// AccountsTableID is the id of the accounts table, which a save or delete replaces.
const AccountsTableID = "accounts-table"

// Accounts is the accounts page (spec §10.3 A1, admins only): the table, and "Add User".
func Accounts(users []app.User) g.Node {
	return layout.Page(layout.Props{
		Title:   "Accounts",
		Current: layout.Accounts,
		Admin:   true,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{
				Title: "Accounts",
				Actions: []g.Node{components.Button(components.ButtonProps{
					Label: "Add User", Variant: components.ButtonPrimary,
					Attrs: []g.Node{hx.Get("/ui/accounts/new"), hx.Target("#" + components.ModalBodyID)},
				})},
			}),
			AccountsTable(users),
		},
	})
}

// AccountsTable is the table of users (A1): name | email | admin (✓/✕) | Edit, Delete.
func AccountsTable(users []app.User) g.Node {
	modal := hx.Target("#" + components.ModalBodyID)
	rows := make([]components.TableRow, 0, len(users))
	for _, u := range users {
		admin := "✕"
		if u.Admin {
			admin = "✓"
		}
		rows = append(rows, components.TableRow{Cells: []g.Node{
			g.Text(u.Name),
			g.Text(u.Email),
			g.Text(admin),
			components.ButtonGroup(
				components.Button(components.ButtonProps{Label: "Edit", Variant: components.ButtonText,
					Attrs: []g.Node{hx.Get(AccountURL(u.ID) + "/edit"), modal}}),
				components.Button(components.ButtonProps{Label: "Delete", Variant: components.ButtonText,
					Attrs: []g.Node{hx.Delete(AccountURL(u.ID)), hx.Confirm("Are you sure you want to delete the user " + u.Name + "?")}}),
			),
		}})
	}
	return components.Table(components.TableProps{
		Columns: []string{"Name", "Email", "Admin", "Actions"},
		Rows:    rows,
		Empty:   "No users.",
		Attrs:   []g.Node{g.Attr("id", AccountsTableID)},
	})
}

// AccountFormProps is what the add and edit modals show.
type AccountFormProps struct {
	ID       int64 // 0 in the add modal
	Name     string
	Email    string
	Admin    bool
	Original app.User // the user as stored when the edit modal opened; only changes are saved
	Error    string
	// FieldErrors are keyed "name", "email", "password", "admin".
	FieldErrors map[string]string
}

// Names of the edit modal's inputs that carry the stored values, so that only the fields
// that changed are saved (A3).
const (
	OrigName  = "orig.name"
	OrigEmail = "orig.email"
	OrigAdmin = "orig.admin"
)

// AccountForm is the add modal (A2: Create is disabled until name, email and password are
// filled) or, with ID, the edit modal (A3: a new password is optional).
func AccountForm(p AccountFormProps) g.Node {
	edit := p.ID != 0
	title, action, save := "Add User", "/ui/accounts", "Create"
	pw := components.TextFieldProps{Label: "Password", Name: "password", ID: "account-password", Type: "password",
		Error: p.FieldErrors["password"], Required: true, Autocomplete: "new-password",
		Hint: "At least " + strconv.Itoa(app.MinPassword) + " characters."}
	var hidden []g.Node
	if edit {
		title, action, save = "Edit User", AccountURL(p.ID), "Save"
		pw.Label, pw.Required, pw.Hint = "New Password", false, "Leave blank to keep the current password."
		orig := ""
		if p.Original.Admin {
			orig = "on"
		}
		hidden = []g.Node{
			components.Hidden(components.HiddenProps{Name: OrigName, Value: p.Original.Name}),
			components.Hidden(components.HiddenProps{Name: OrigEmail, Value: p.Original.Email}),
			components.Hidden(components.HiddenProps{Name: OrigAdmin, Value: orig}),
		}
	}
	fields := append(hidden,
		components.TextField(components.TextFieldProps{Label: "Name", Name: "name", ID: "account-name", Value: p.Name,
			Error: p.FieldErrors["name"], Required: true}),
		components.TextField(components.TextFieldProps{Label: "Email", Name: "email", ID: "account-email", Type: "email", Value: p.Email,
			Error: p.FieldErrors["email"], Required: true, Autocomplete: "off"}),
		components.TextField(pw),
		components.Switch(components.SwitchProps{Label: "Admin", Name: "admin", ID: "account-admin", Checked: p.Admin,
			Error: p.FieldErrors["admin"]}),
	)
	return components.ModalContent(components.ModalContentProps{
		Title: title,
		Body: []g.Node{components.Form(components.FormProps{
			Action: action,
			Error:  p.Error,
			Attrs: []g.Node{hx.Post(action), hx.Target("#" + components.ModalBodyID),
				g.Attr("x-data", "requiredFields"), g.Attr("x-on:input", "update")},
			Fields: fields,
			Footer: components.ModalFooter(components.ModalFooterProps{
				End: []g.Node{
					components.CloseModalButton(),
					components.Button(components.ButtonProps{Label: save, Variant: components.ButtonPrimary, Submit: true,
						Attrs: []g.Node{g.Attr("x-bind:disabled", "incomplete")}}),
				},
			}),
		})},
	})
}

// AccountFormPage shows an account form as a page of its own, for a browser without
// JavaScript.
func AccountFormPage(p AccountFormProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "Accounts",
		Current: layout.Accounts,
		Admin:   true,
		Body:    []g.Node{components.Card(components.CardProps{Body: []g.Node{AccountForm(p)}})},
	})
}

// AccountURL is the UI's URL of the user with id.
func AccountURL(id int64) string { return "/ui/accounts/" + strconv.FormatInt(id, 10) }

// MyAccountFormID is the id of the My Account form, which a save replaces.
const MyAccountFormID = "my-account-form"

// MyAccountProps is what My Account shows.
type MyAccountProps struct {
	Admin bool
	Name  string
	Email string
	Error string
	// FieldErrors are keyed "name", "email", "currentPassword", "newPassword",
	// "confirmPassword".
	FieldErrors map[string]string
}

// MyAccount is the My Account page (spec §10.3 M1).
func MyAccount(p MyAccountProps) g.Node {
	return layout.Page(layout.Props{
		Title:   "My Account",
		Current: layout.MyAccount,
		Admin:   p.Admin,
		Body: []g.Node{
			components.PageHeader(components.PageHeaderProps{Title: "My Account"}),
			components.Card(components.CardProps{Body: []g.Node{MyAccountForm(p)}}),
		},
	})
}

// MyAccountForm is the form of My Account: name, email and, to change the password, the
// current one and the new one twice. While the two differ an inline error shows and Save
// is disabled (Alpine component "passwordConfirm"); the server checks it too. Only the
// fields that changed are saved; the password fields always come back empty.
func MyAccountForm(p MyAccountProps) g.Node {
	pw := func(label, name, auto, hint string) g.Node {
		return components.TextField(components.TextFieldProps{Label: label, Name: name, ID: "my-" + name, Type: "password",
			Error: p.FieldErrors[name], Autocomplete: auto, Hint: hint})
	}
	return components.Form(components.FormProps{
		Action: "/my-account",
		Error:  p.Error,
		Attrs: []g.Node{g.Attr("id", MyAccountFormID), hx.Post("/my-account"), hx.Target("this"), hx.Swap("outerHTML"),
			g.Attr("x-data", "passwordConfirm"), g.Attr("x-on:input", "check")},
		Fields: []g.Node{
			components.TextField(components.TextFieldProps{Label: "Name", Name: "name", ID: "my-name", Value: p.Name,
				Error: p.FieldErrors["name"], Required: true, Autocomplete: "name"}),
			components.TextField(components.TextFieldProps{Label: "Email", Name: "email", ID: "my-email", Type: "email", Value: p.Email,
				Error: p.FieldErrors["email"], Required: true, Autocomplete: "email"}),
			pw("Current Password", "currentPassword", "current-password", "Needed only to change the password."),
			pw("New Password", "newPassword", "new-password", "Leave blank to keep the current password."),
			pw("Confirm New Password", "confirmPassword", "new-password", ""),
			components.InlineError(components.InlineErrorProps{
				Message: "The new passwords do not match.",
				Attrs:   []g.Node{g.Attr("x-bind:hidden", "matches")},
			}),
		},
		Actions: []g.Node{components.Button(components.ButtonProps{Label: "Save", Variant: components.ButtonPrimary, Submit: true,
			Attrs: []g.Node{g.Attr("x-bind:disabled", "mismatch")}})},
	})
}
