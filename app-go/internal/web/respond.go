package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/logx"
)

// isHTMX reports whether htmx sent the request.
func isHTMX(r *http.Request) bool { return r.Header.Get("HX-Request") == "true" }

// redirect sends the browser to url (spec §10.2): htmx gets HX-Redirect on a 200, because
// it ignores the headers of 3xx responses; a plain request gets a 303.
func redirect(w http.ResponseWriter, r *http.Request, url string) {
	if isHTMX(r) {
		w.Header().Set("HX-Redirect", url)
		w.WriteHeader(http.StatusOK)
		return
	}
	http.Redirect(w, r, url, http.StatusSeeOther)
}

// Toast types (spec §10.2; "info" and "warning" also exist).
const (
	toastSuccess = "success"
	toastError   = "error"
)

// toast asks the page to show a toast once htmx has handled the response, through
// HX-Trigger (spec §10.2). The event is raised on <body> ("target"), because the element
// that sent the request may have been swapped out by then, and an event on a detached
// element never reaches the listener in app.js.
func toast(w http.ResponseWriter, typ, title, message string) {
	b, err := json.Marshal(map[string]any{"toast": map[string]string{
		"target": "body", "type": typ, "title": title, "message": message,
	}})
	if err != nil {
		panic(err) // strings always marshal
	}
	w.Header().Set("HX-Trigger", string(b))
}

// fail answers a request whose use case failed with anything but a form error
// (spec §10.2): an *app.Error with its status and message, anything else as a logged
// 500 with a generic message. htmx requests leave the page as it is (HX-Reswap: none)
// and show an error toast; plain requests get a short text page.
func (u *ui) fail(w http.ResponseWriter, r *http.Request, err error) {
	status, msg := http.StatusInternalServerError, "Something went wrong. Please try again."
	var appErr *app.Error
	if errors.As(err, &appErr) {
		status, msg = appErr.StatusCode(), appErr.Message
	} else {
		u.logger.LogAttrs(r.Context(), slog.LevelError, "request failed: "+err.Error(),
			slog.String(logx.AttrHTTPMethod, r.Method),
			slog.String(logx.AttrURLPath, r.URL.Path),
			slog.String(logx.AttrErrorType, fmt.Sprintf("%T", err)),
			slog.String(logx.AttrExceptionMessage, err.Error()),
		)
	}
	if !isHTMX(r) {
		http.Error(w, msg, status)
		return
	}
	w.Header().Set("HX-Reswap", "none")
	toast(w, toastError, "Error", msg)
	w.WriteHeader(status)
}
