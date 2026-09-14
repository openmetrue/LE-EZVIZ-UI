package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func sessionValue(expiry time.Time) string {
	exp := fmt.Sprintf("%d", expiry.Unix())
	mac := hmac.New(sha256.New, []byte(cfgCopy().SessionSecret))
	mac.Write([]byte(exp))
	return exp + "." + hex.EncodeToString(mac.Sum(nil))
}

func validSession(r *http.Request) bool {
	c, err := r.Cookie(sessionCookie)
	if err != nil {
		return false
	}
	parts := strings.SplitN(c.Value, ".", 2)
	if len(parts) != 2 {
		return false
	}
	var exp int64
	if _, err := fmt.Sscanf(parts[0], "%d", &exp); err != nil || time.Now().Unix() > exp {
		return false
	}
	expected := sessionValue(time.Unix(exp, 0))
	return hmac.Equal([]byte(c.Value), []byte(expected))
}

func validToken(r *http.Request) bool {
	t := r.URL.Query().Get("token")
	tok := cfgCopy().DeviceToken
	return t != "" && tok != "" && hmac.Equal([]byte(t), []byte(tok))
}

func setSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    sessionValue(time.Now().Add(30 * 24 * time.Hour)),
		Path:     *basePath + "/",
		HttpOnly: true,
		Secure:   true,
		SameSite: http.SameSiteLaxMode,
	})
}

func clearSession(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: sessionCookie, Value: "", Path: *basePath + "/", MaxAge: -1})
}

var (
	loginMu    sync.Mutex
	loginFails = map[string][]time.Time{}
)

func loginAllowed(ip string) bool {
	loginMu.Lock()
	defer loginMu.Unlock()
	cutoff := time.Now().Add(-10 * time.Minute)
	var recent []time.Time
	for _, t := range loginFails[ip] {
		if t.After(cutoff) {
			recent = append(recent, t)
		}
	}
	loginFails[ip] = recent
	return len(recent) < 5
}

func loginFailed(ip string) {
	loginMu.Lock()
	defer loginMu.Unlock()
	loginFails[ip] = append(loginFails[ip], time.Now())
}

func passwordOK(pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(cfgCopy().PasswordHash), []byte(pw)) == nil
}

func bcryptHash(pw string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(pw), bcrypt.DefaultCost)
	return string(b), err
}

func clientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	return r.RemoteAddr
}

func securityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}

func auth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r) {
			http.Redirect(w, r, *basePath+"/login", http.StatusSeeOther)
			return
		}
		next(w, r)
	}
}

func authOrToken(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !validSession(r) && !validToken(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}
