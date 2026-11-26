// Package web is the UI's HTTP layer: routes, handlers and the static file server
// (spec §2.1, §10).
package web

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"

	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/views/layout"
	"github.com/laposa/tmock/app-go/static"
)

// Deps are what the UI handlers need.
type Deps struct {
	App      *app.App
	Sessions *auth.Sessions
	Logger   *slog.Logger // nil means slog.Default()
}

// ui holds the handlers' dependencies.
type ui struct {
	app      *app.App
	sessions *auth.Sessions
	logger   *slog.Logger
}

// Handler returns the UI handler: the pages of spec §10.3 built so far, login and logout,
// and /static/*. Everything else is 404.
func Handler(d Deps) http.Handler {
	u := &ui{app: d.App, sessions: d.Sessions, logger: d.Logger}
	if u.logger == nil {
		u.logger = slog.Default()
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /login", u.loginPage)
	mux.HandleFunc("POST /login", u.login)
	mux.HandleFunc("POST /logout", u.logout)
	u.registerServices(mux)
	u.registerScenarios(mux)
	// Placeholders until each page's iteration (spec §15.3).
	mux.Handle("GET /{$}", u.placeholder(layout.Clients, "Clients", "I07", false))
	mux.Handle("GET /accounts", u.placeholder(layout.Accounts, "Accounts", "I08", true))
	mux.Handle("GET /my-account", u.placeholder(layout.MyAccount, "My Account", "I08", false))
	mux.Handle("GET /static/", http.StripPrefix("/static/", staticFiles()))
	return mux
}

// staticFiles serves the embedded assets. Vendored files have the version in their
// name, so browsers may cache them for good; our own files are revalidated each time.
// Embedded files have no modification time, so each gets an ETag from its content;
// the file server then answers If-None-Match with 304 Not Modified.
func staticFiles() http.Handler {
	files := http.FileServerFS(static.FS)
	etags := contentETags(static.FS)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "", r.URL.Path == "vendor/VERSIONS", strings.HasSuffix(r.URL.Path, "/"):
			http.NotFound(w, r) // no directory listings, no internal files
			return
		case strings.HasPrefix(r.URL.Path, "vendor/"):
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		default:
			w.Header().Set("Cache-Control", "no-cache")
		}
		if etag, ok := etags[r.URL.Path]; ok {
			w.Header().Set("ETag", etag)
		}
		files.ServeHTTP(w, r)
	})
}

// contentETags returns a strong ETag for every file in fsys, keyed by its path.
func contentETags(fsys fs.FS) map[string]string {
	etags := map[string]string{}
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := fs.ReadFile(fsys, path)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(b)
		etags[path] = `"` + hex.EncodeToString(sum[:16]) + `"`
		return nil
	})
	if err != nil {
		panic("static: cannot read embedded files: " + err.Error()) // embedded, so a build bug
	}
	return etags
}

// render writes a gomponents node as an HTML response. Rendering into a buffer first
// means a rendering error becomes a clean 500 instead of a half-written page. Pages are
// not cached, so the back button after logout does not show a logged-in page.
func (u *ui) render(w http.ResponseWriter, r *http.Request, status int, n g.Node) {
	var buf bytes.Buffer
	if err := n.Render(&buf); err != nil {
		u.logger.ErrorContext(r.Context(), "rendering page failed",
			slog.String(logx.AttrURLPath, r.URL.Path), slog.String(logx.AttrExceptionMessage, err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_, _ = buf.WriteTo(w)
}
