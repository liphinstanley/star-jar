package main

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	familyCookie = "sj_family" // this device belongs to the family: kid pages
	parentCookie = "sj_parent" // a parent typed the PIN: /parent pages
	familyTTL    = 365 * 24 * time.Hour
	parentTTL    = 15 * time.Minute
)

var (
	errWrongSecret  = inputError("that's not right, try again")
	errTooManyTries = inputError("too many tries, wait 5 minutes")
)

type auth struct {
	familyPassword string
	parentPIN      string
	key            []byte // signs cookies, so nobody can make their own
	loginLimit     limiter
	pinLimit       limiter
}

func newAuth(password, pin, keyHex string) (*auth, error) {
	if password == "" || pin == "" {
		return nil, errors.New("FAMILY_PASSWORD and PARENT_PIN must be set")
	}
	a := &auth{familyPassword: password, parentPIN: pin}
	if keyHex == "" {
		a.key = make([]byte, 32)
		rand.Read(a.key)
		log.Println("SESSION_KEY not set: using a random key, every device is signed out on restart")
		return a, nil
	}
	key, err := hex.DecodeString(keyHex)
	if err != nil || len(key) < 32 {
		return nil, errors.New("SESSION_KEY must be at least 64 hex characters (openssl rand -hex 32)")
	}
	a.key = key
	return a, nil
}

// token is "<expiry>.<signature>"; the signature proves we made it and nobody changed the expiry
func (a *auth) token(name string, exp int64) string {
	mac := hmac.New(sha256.New, a.key)
	fmt.Fprintf(mac, "%s|%d", name, exp)
	return strconv.FormatInt(exp, 10) + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func (a *auth) valid(r *http.Request, name string) bool {
	c, err := r.Cookie(name)
	if err != nil {
		return false
	}
	expStr, _, ok := strings.Cut(c.Value, ".")
	exp, err := strconv.ParseInt(expStr, 10, 64)
	if !ok || err != nil || time.Now().Unix() > exp {
		return false
	}
	return hmac.Equal([]byte(c.Value), []byte(a.token(name, exp)))
}

func (a *auth) setCookie(w http.ResponseWriter, r *http.Request, name string, ttl time.Duration) {
	exp := time.Now().Add(ttl)
	http.SetCookie(w, &http.Cookie{
		Name:     name,
		Value:    a.token(name, exp.Unix()),
		Path:     "/",
		Expires:  exp,
		HttpOnly: true, // JavaScript can't read it
		Secure:   isHTTPS(r),
		SameSite: http.SameSiteLaxMode, // other sites can't POST with it
	})
}

func clearCookie(w http.ResponseWriter, name string) {
	http.SetCookie(w, &http.Cookie{Name: name, Path: "/", MaxAge: -1})
}

// isHTTPS is true behind Cloud Run too, which ends TLS and forwards plain HTTP
func isHTTPS(r *http.Request) bool {
	return r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https"
}

// check compares what was typed with the secret, counting wrong tries
func (a *auth) check(l *limiter, given, want string) error {
	if l.locked() {
		return errTooManyTries
	}
	ok := subtle.ConstantTimeCompare([]byte(given), []byte(want)) == 1
	l.record(ok)
	if !ok {
		return errWrongSecret
	}
	return nil
}

func (a *auth) requireFamily(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.valid(r, familyCookie) {
			redirect(w, r, "/login")
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (a *auth) requireParent(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !a.valid(r, parentCookie) {
			redirect(w, r, "/parent/pin")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// redirect moves the whole page; for htmx requests (polling, boosted forms)
// a normal redirect would only swap part of the page, so ask htmx to do it
func redirect(w http.ResponseWriter, r *http.Request, to string) {
	if r.Header.Get("HX-Request") == "true" {
		w.Header().Set("HX-Redirect", to)
		return
	}
	http.Redirect(w, r, to, http.StatusSeeOther)
}

type authPage struct {
	Error string
}

func (a *auth) loginPage(w http.ResponseWriter, r *http.Request) {
	render(w, "login.html", authPage{Error: r.URL.Query().Get("error")})
}

func (a *auth) login(w http.ResponseWriter, r *http.Request) {
	if err := a.check(&a.loginLimit, r.FormValue("password"), a.familyPassword); err != nil {
		log.Printf("login failed: %v (from %s)", err, r.Header.Get("X-Forwarded-For"))
		http.Redirect(w, r, "/login?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	a.setCookie(w, r, familyCookie, familyTTL)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (a *auth) logout(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, familyCookie)
	clearCookie(w, parentCookie)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (a *auth) pinPage(w http.ResponseWriter, r *http.Request) {
	render(w, "pin.html", authPage{Error: r.URL.Query().Get("error")})
}

func (a *auth) unlock(w http.ResponseWriter, r *http.Request) {
	if err := a.check(&a.pinLimit, r.FormValue("pin"), a.parentPIN); err != nil {
		log.Printf("PIN failed: %v (from %s)", err, r.Header.Get("X-Forwarded-For"))
		http.Redirect(w, r, "/parent/pin?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	a.setCookie(w, r, parentCookie, parentTTL)
	http.Redirect(w, r, "/parent", http.StatusSeeOther)
}

func (a *auth) lock(w http.ResponseWriter, r *http.Request) {
	clearCookie(w, parentCookie)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

// limiter locks a form for a while after too many wrong tries
type limiter struct {
	mu    sync.Mutex
	fails int
	until time.Time
}

const (
	maxFails = 5
	lockout  = 5 * time.Minute
)

func (l *limiter) locked() bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	return time.Now().Before(l.until)
}

func (l *limiter) record(ok bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if ok {
		l.fails = 0
		return
	}
	l.fails++
	if l.fails >= maxFails {
		l.until = time.Now().Add(lockout)
		l.fails = 0
	}
}
