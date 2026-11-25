package web

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"maps"
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
// HX-Trigger (spec §10.2).
func toast(w http.ResponseWriter, typ, title, message string) {
	trigger(w, "toast", map[string]string{"type": typ, "title": title, "message": message})
}

// closeModal asks the page to close the modal (spec §10.2).
func closeModal(w http.ResponseWriter) { trigger(w, "modal:close", map[string]string{}) }

// trigger adds the event name with detail to the response's HX-Trigger header, keeping
// the events already there. The event is raised on <body> ("target"), because the
// element that sent the request may have been swapped out by then, and an event on a
// detached element never reaches the listeners in app.js.
func trigger(w http.ResponseWriter, name string, detail map[string]string) {
	events := map[string]map[string]string{}
	if cur := w.Header().Get("HX-Trigger"); cur != "" {
		if err := json.Unmarshal([]byte(cur), &events); err != nil {
			panic("web: HX-Trigger is not ours: " + err.Error())
		}
	}
	d := map[string]string{"target": "body"}
	maps.Copy(d, detail)
	events[name] = d
	b, err := json.Marshal(events)
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
