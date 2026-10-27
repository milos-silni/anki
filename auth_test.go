package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
)

func newTestApp(t *testing.T, password string) http.Handler {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "cards.json"))
	if err != nil {
		t.Fatal(err)
	}
	app := &App{store: store, pages: loadPages()}
	if password != "" {
		app.auth = NewAuth(password, false)
	}
	return app.handler()
}

func do(h http.Handler, method, target string, form url.Values, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	var body *strings.Reader
	if form != nil {
		body = strings.NewReader(form.Encode())
	} else {
		body = strings.NewReader("")
	}
	r := httptest.NewRequest(method, target, body)
	if form != nil {
		r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	}
	for _, c := range cookies {
		r.AddCookie(c)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func sessionFrom(t *testing.T, w *httptest.ResponseRecorder) *http.Cookie {
	t.Helper()
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.Value != "" {
			return c
		}
	}
	t.Fatal("no session cookie set")
	return nil
}

func TestAuthRedirectsToLogin(t *testing.T) {
	h := newTestApp(t, "geheim")
	for _, path := range []string{"/", "/cards?q=Hund", "/export"} {
		w := do(h, "GET", path, nil)
		if w.Code != http.StatusSeeOther || !strings.HasPrefix(w.Header().Get("Location"), "/login") {
			t.Errorf("GET %s: got %d %q, want redirect to /login", path, w.Code, w.Header().Get("Location"))
		}
	}
	if w := do(h, "POST", "/delete", url.Values{"id": {"1"}}); w.Header().Get("Location") != "/login" {
		t.Errorf("POST /delete not blocked: %d %q", w.Code, w.Header().Get("Location"))
	}
	for _, path := range []string{"/static/style.css", "/static/manifest.json", "/static/icons/icon-192.png", "/sw.js"} {
		if w := do(h, "GET", path, nil); w.Code != http.StatusOK {
			t.Errorf("%s should be public, got %d", path, w.Code)
		}
	}
	if w := do(h, "GET", "/login", nil); w.Code != http.StatusOK {
		t.Errorf("login page: got %d", w.Code)
	}
}

func TestAuthLoginFlow(t *testing.T) {
	h := newTestApp(t, "geheim")

	w := do(h, "POST", "/login", url.Values{"password": {"falsch"}})
	if w.Code != http.StatusUnauthorized || len(w.Result().Cookies()) != 0 {
		t.Fatalf("wrong password: got %d with cookies %v", w.Code, w.Result().Cookies())
	}

	w = do(h, "POST", "/login", url.Values{"password": {"geheim"}, "next": {"/cards?q=Hund"}})
	if w.Code != http.StatusSeeOther || w.Header().Get("Location") != "/cards?q=Hund" {
		t.Fatalf("login: got %d %q", w.Code, w.Header().Get("Location"))
	}
	session := sessionFrom(t, w)
	if !session.HttpOnly || session.SameSite != http.SameSiteLaxMode {
		t.Errorf("cookie flags: %+v", session)
	}

	if w := do(h, "GET", "/", nil, session); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "Log out") {
		t.Fatalf("logged-in home: got %d", w.Code)
	}

	w = do(h, "POST", "/logout", nil, session)
	for _, c := range w.Result().Cookies() {
		if c.Name == sessionCookie && c.MaxAge >= 0 {
			t.Errorf("logout did not clear cookie: %+v", c)
		}
	}
}

func TestAuthRejectsForgedSessions(t *testing.T) {
	h := newTestApp(t, "geheim")
	other := NewAuth("anders", false)
	for _, v := range []string{
		"", "garbage", "9999999999.deadbeef",
		"9999999999." + other.sign(9999999999),  // signed with another password
		"1." + NewAuth("geheim", false).sign(1), // expired
	} {
		w := do(h, "GET", "/", nil, &http.Cookie{Name: sessionCookie, Value: v})
		if w.Code != http.StatusSeeOther {
			t.Errorf("cookie %q accepted (status %d)", v, w.Code)
		}
	}
}

func TestSafeNext(t *testing.T) {
	for in, want := range map[string]string{
		"/cards?q=x":         "/cards?q=x",
		"":                   "/",
		"https://evil.com":   "/",
		"//evil.com":         "/",
		"/\\evil.com":        "/",
		"javascript:alert()": "/",
	} {
		if got := safeNext(in); got != want {
			t.Errorf("safeNext(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestNoPasswordMeansNoAuth(t *testing.T) {
	h := newTestApp(t, "")
	if w := do(h, "GET", "/", nil); w.Code != http.StatusOK || strings.Contains(w.Body.String(), "Log out") {
		t.Fatalf("got %d", w.Code)
	}
}
