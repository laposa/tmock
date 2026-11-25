package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// AlertType is the kind of message, with the colours of Vuetify's v-alert types.
type AlertType int

const (
	AlertError AlertType = iota
	AlertWarning
	AlertInfo
	AlertSuccess
)

// AlertProps configures an Alert.
type AlertProps struct {
	Type    AlertType
	Message string
	Attrs   []g.Node
}

// Alert is an inline message, such as a failed login. Errors are announced at once
// (role="alert"), the others politely (role="status").
func Alert(p AlertProps) g.Node {
	c, role := "alert alert--error", "alert"
	switch p.Type {
	case AlertWarning:
		c = "alert alert--warning"
	case AlertInfo:
		c, role = "alert alert--info", "status"
	case AlertSuccess:
		c, role = "alert alert--success", "status"
	}
	return h.Div(h.Class(c), h.Role(role), attrs(p.Attrs), g.Text(p.Message))
}
