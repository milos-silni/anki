package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	sessionCookie = "anki_session"
	sessionTTL    = 30 * 24 * time.Hour
	loginDelay    = time.Second
)

// Auth protects the app with a single shared password. Sessions are stateless:
// the cookie holds an expiry time signed with a key derived from the password,
// so they survive restarts and changing the password logs everyone out.
type Auth struct {
	passHash [32]byte
	key      []byte
	secure   bool
	mu       sync.Mutex // serialises login attempts to slow down guessing
}

func NewAuth(password string, secure bool) *Auth {
	key := sha256.Sum256([]byte("anki-session:" + password))
	return &Auth{passHash: sha256.Sum256([]byte(password)), key: key[:], secure: secure}
}

func (a *Auth) sign(expires int64) string {
	m := hmac.New(sha256.New, a.key)
	m.Write([]byte(strconv.FormatInt(expires, 10)))
	return hex.EncodeToString(m.Sum(nil))
}

func (a *Auth) validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	exp, sig, ok := strings.Cut(c.Value, ".")
	if !ok {
		return false
	}
	expires, err := strconv.ParseInt(exp, 10, 64)
	if err != nil || time.Now().Unix() > expires {
		return false
	}
	return hmac.Equal([]byte(sig), []byte(a.sign(expires)))
}

func (a *Auth) checkPassword(password string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	h := sha256.Sum256([]byte(password))
	if subtle.ConstantTimeCompare(h[:], a.passHash[:]) == 1 {
		return true
	}
	time.Sleep(loginDelay)
	return false
}

func (a *Auth) setCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: sessionCookie, Value: value, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: a.secure, SameSite: http.SameSiteLaxMode,
	})
}

// Middleware sends visitors without a valid session to the login page.
func (a *Auth) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/login" || r.URL.Path == "/sw.js" || strings.HasPrefix(r.URL.Path, "/static/") || a.validSession(r) {
			next.ServeHTTP(w, r)
			return
		}
		target := "/login"
		if r.Method == http.MethodGet && r.URL.RequestURI() != "/" {
			target += "?next=" + url.QueryEscape(r.URL.RequestURI())
		}
		http.Redirect(w, r, target, http.StatusSeeOther)
	})
}

// safeNext only allows redirects to paths on this site.
func safeNext(next string) string {
	if !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.HasPrefix(next, "/\\") {
		return "/"
	}
	return next
}

func (app *App) login(w http.ResponseWriter, r *http.Request) {
	next := safeNext(r.FormValue("next"))
	if app.auth.validSession(r) {
		http.Redirect(w, r, next, http.StatusSeeOther)
		return
	}
	data := map[string]any{"Next": next}
	if r.Method == http.MethodPost {
		if app.auth.checkPassword(r.FormValue("password")) {
			expires := time.Now().Add(sessionTTL).Unix()
			value := strconv.FormatInt(expires, 10) + "." + app.auth.sign(expires)
			app.auth.setCookie(w, value, int(sessionTTL.Seconds()))
			http.Redirect(w, r, next, http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8") // must precede WriteHeader
		w.WriteHeader(http.StatusUnauthorized)
		data["Error"] = "Wrong password."
	}
	app.render(w, "login", data)
}

func (app *App) logout(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodPost {
		app.auth.setCookie(w, "", -1)
	}
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}
