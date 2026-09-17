package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestSetSessionPersistsInBrowser(t *testing.T) {
	orig := cfg
	origBase := *basePath
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = orig
		cfgMu.Unlock()
		*basePath = origBase
	})
	cfgMu.Lock()
	cfg.SessionSecret = "test-secret"
	cfgMu.Unlock()
	*basePath = "/ezviz"

	rec := httptest.NewRecorder()
	setSession(rec)
	cookies := rec.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies=%d", len(cookies))
	}
	c := cookies[0]
	if c.Name != sessionCookie {
		t.Fatalf("name=%s", c.Name)
	}
	if c.MaxAge < int((300 * 24 * time.Hour / time.Second)) {
		t.Fatalf("MaxAge=%d, want ~1 year so Safari keeps it after quit", c.MaxAge)
	}
	if c.Expires.Before(time.Now().Add(300 * 24 * time.Hour)) {
		t.Fatalf("Expires=%s", c.Expires)
	}
	req := httptest.NewRequest(http.MethodGet, "/ezviz/", nil)
	req.AddCookie(c)
	if !validSession(req) {
		t.Fatal("fresh cookie should be valid")
	}
}
