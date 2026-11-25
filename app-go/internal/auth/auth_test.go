package auth_test

import (
	"context"
	"encoding/base64"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/auth/authtest"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/logxtest"
	"github.com/laposa/tmock/app-go/internal/store"
)

var secret = []byte("0123456789abcdef0123456789abcdef")

func newSessions(t *testing.T, secure bool) (*auth.Sessions, *authtest.Store, *logxtest.Recorder) {
	t.Helper()
	st := authtest.New()
	st.AddUser(store.User{ID: 1, Name: "Ann", Email: "ann@example.com", Admin: true})
	st.AddUser(store.User{ID: 2, Name: "Bob", Email: "bob@example.com"})
	logs, logger := logxtest.New()
	return auth.New(st, auth.Options{Secret: secret, Secure: secure, Logger: logger}), st, logs
}

// login starts a session for userID and returns its cookie.
func login(t *testing.T, s *auth.Sessions, userID int64) *http.Cookie {
	t.Helper()
	rec := httptest.NewRecorder()
	if err := s.Start(context.Background(), rec, userID); err != nil {
		t.Fatal(err)
	}
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies %v", cookies)
	}
	return cookies[0]
}

func request(c *http.Cookie) *http.Request {
	r := httptest.NewRequest("GET", "/", nil)
	if c != nil {
		r.AddCookie(c)
	}
	return r
}

func TestCookieAttributes(t *testing.T) {
	for _, secure := range []bool{false, true} {
		s, _, _ := newSessions(t, secure)
		rec := httptest.NewRecorder()
		if err := s.Start(context.Background(), rec, 1); err != nil {
			t.Fatal(err)
		}
		header := rec.Header().Get("Set-Cookie")
		for _, want := range []string{"tmock_session=", "Path=/", "Max-Age=86400", "HttpOnly", "SameSite=Lax"} {
			if !strings.Contains(header, want) {
				t.Errorf("secure=%v: Set-Cookie %q lacks %q", secure, header, want)
			}
		}
		if strings.Contains(header, "Secure") != secure {
			t.Errorf("secure=%v: Set-Cookie %q", secure, header)
		}
		value := rec.Result().Cookies()[0].Value
		if raw, err := base64.RawURLEncoding.DecodeString(value); err != nil || len(raw) != 32 {
			t.Errorf("cookie value %q is not 32 base64url bytes", value)
		}
	}
}

func TestStartUserEnd(t *testing.T) {
	s, st, logs := newSessions(t, false)
	c := login(t, s, 2)

	u, err := s.User(request(c))
	if err != nil || u.ID != 2 || u.Name != "Bob" || u.Admin {
		t.Fatalf("User: %+v, %v", u, err)
	}

	// Each login is a new session.
	if other := login(t, s, 2); other.Value == c.Value {
		t.Error("second login reused the session")
	}
	if st.Len() != 2 {
		t.Errorf("%d sessions", st.Len())
	}

	rec := httptest.NewRecorder()
	if err := s.End(rec, request(c)); err != nil {
		t.Fatal(err)
	}
	if h := rec.Header().Get("Set-Cookie"); !strings.Contains(h, "tmock_session=;") || !strings.Contains(h, "Max-Age=0") {
		t.Errorf("logout cookie %q", h)
	}
	if _, err := s.User(request(c)); err != auth.ErrNoSession {
		t.Errorf("after logout: %v", err)
	}
	events := logs.Events(logx.EventLogout)
	if len(events) != 1 || events[0].Level != slog.LevelInfo || events[0].Attrs[logx.AttrUserID] != "2" {
		t.Errorf("logout events %+v", events)
	}

	// Logging out again, or without a cookie, is harmless and logs nothing.
	for _, r := range []*http.Request{request(c), request(nil)} {
		if err := s.End(httptest.NewRecorder(), r); err != nil {
			t.Error(err)
		}
	}
	if n := len(logs.Events(logx.EventLogout)); n != 1 {
		t.Errorf("%d logout events", n)
	}
}

func TestUserWithoutValidCookie(t *testing.T) {
	s, _, _ := newSessions(t, false)
	for name, c := range map[string]*http.Cookie{
		"no cookie": nil,
		"garbage":   {Name: auth.CookieName, Value: "not base64!"},
		"too short": {Name: auth.CookieName, Value: base64.RawURLEncoding.EncodeToString([]byte("short"))},
		"unknown":   {Name: auth.CookieName, Value: base64.RawURLEncoding.EncodeToString(make([]byte, 32))},
	} {
		if _, err := s.User(request(c)); err != auth.ErrNoSession {
			t.Errorf("%s: %v", name, err)
		}
	}
}

// The stored id is an HMAC under the secret: the same cookie is unknown to a server with
// another secret.
func TestSessionIDDependsOnSecret(t *testing.T) {
	s, st, _ := newSessions(t, false)
	c := login(t, s, 1)
	other := auth.New(st, auth.Options{Secret: []byte("another-secret-another-secret-00")})
	if _, err := other.User(request(c)); err != auth.ErrNoSession {
		t.Errorf("other secret: %v", err)
	}
}

// Verify item: session expiry, with synctest's fake clock.
func TestSessionExpiresAfter24Hours(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, _, _ := newSessions(t, false)
		c := login(t, s, 1)
		time.Sleep(auth.Lifetime - time.Second)
		if _, err := s.User(request(c)); err != nil {
			t.Fatalf("just before expiry: %v", err)
		}
		time.Sleep(time.Second)
		if _, err := s.User(request(c)); err != auth.ErrNoSession {
			t.Errorf("at expiry: %v", err)
		}
	})
}

// Verify item: a deleted user's session stops working; an admin flag change applies on
// the next request.
func TestUserChangesApplyAtOnce(t *testing.T) {
	s, st, _ := newSessions(t, false)
	c := login(t, s, 2)
	st.AddUser(store.User{ID: 2, Name: "Bob", Email: "bob@example.com", Admin: true})
	if u, _ := s.User(request(c)); !u.Admin {
		t.Error("admin flag not current")
	}
	st.DeleteUser(2)
	if _, err := s.User(request(c)); err != auth.ErrNoSession {
		t.Errorf("deleted user: %v", err)
	}
}

func TestSweeper(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, st, logs := newSessions(t, false)
		login(t, s, 1)
		time.Sleep(time.Hour)
		login(t, s, 2) // expires an hour after the first

		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() { defer close(done); s.RunSweeper(ctx) }()

		synctest.Wait() // the sweep at start: nothing has expired
		if st.Len() != 2 {
			t.Fatalf("%d sessions after the first sweep", st.Len())
		}
		time.Sleep(auth.Lifetime - time.Hour) // the first session has expired now
		synctest.Wait()
		if st.Len() != 1 {
			t.Errorf("%d sessions, want 1", st.Len())
		}
		time.Sleep(auth.SweepInterval)
		synctest.Wait()
		if st.Len() != 0 {
			t.Errorf("%d sessions, want 0", st.Len())
		}
		cancel()
		<-done

		events := logs.Events(logx.EventSessionsSwept)
		var deleted []string
		for _, e := range events {
			if e.Level != slog.LevelDebug {
				t.Errorf("level %v", e.Level)
			}
			deleted = append(deleted, e.Attrs[logx.AttrSessionsDeleted])
		}
		// One sweep at start, then hourly for 24 hours.
		if len(deleted) != 25 || deleted[0] != "0" || deleted[23] != "1" || deleted[24] != "1" {
			t.Errorf("sweeps: %v", deleted)
		}
	})
}

func TestGuards(t *testing.T) {
	s, _, _ := newSessions(t, false)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, found := auth.UserFrom(r.Context())
		if !found {
			t.Error("no user in context")
		}
		w.Header().Set("X-User", u.Name)
	})
	admin, user := login(t, s, 1), login(t, s, 2)
	cases := []struct {
		name   string
		h      http.Handler
		cookie *http.Cookie
		status int
		body   string
	}{
		{"user guard, anonymous", s.RequireUser(ok), nil, 401, `{"status":401,"message":"Not logged in"}`},
		{"user guard, user", s.RequireUser(ok), user, 200, ""},
		{"admin guard, anonymous", s.RequireAdmin(ok), nil, 401, `"Not logged in"`},
		{"admin guard, user", s.RequireAdmin(ok), user, 403, `{"status":403,"message":"Only admins may do this"}`},
		{"admin guard, admin", s.RequireAdmin(ok), admin, 200, ""},
	}
	for _, tc := range cases {
		rec := httptest.NewRecorder()
		tc.h.ServeHTTP(rec, request(tc.cookie))
		if rec.Code != tc.status || !strings.Contains(rec.Body.String(), tc.body) {
			t.Errorf("%s: %d %s", tc.name, rec.Code, rec.Body.String())
		}
	}
}

// The guard records the user for the request log.
func TestGuardSetsLogUser(t *testing.T) {
	s, _, _ := newSessions(t, false)
	c := login(t, s, 2)
	logs, logger := logxtest.New()
	h := logx.RequestLog(logger, s.RequireUser(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {})))
	h.ServeHTTP(httptest.NewRecorder(), request(c))
	if e := logs.Events(logx.EventHTTPRequest); len(e) != 1 || e[0].Attrs[logx.AttrUserID] != "2" {
		t.Errorf("request events %+v", e)
	}
}
