package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSetupFoldsHidePasswordForms(t *testing.T) {
	orig := streamer
	streamer = NewStreamer()
	t.Cleanup(func() { streamer = orig })
	origCfg := cfg
	origBase := *basePath
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = origCfg
		cfgMu.Unlock()
		*basePath = origBase
	})
	*basePath = "/ezviz"
	cfgMu.Lock()
	cfg.Email = "cam@example.test"
	cfg.Password = "secret"
	cfg.Region = "Russia"
	cfgMu.Unlock()

	rec := httptest.NewRecorder()
	handleSetup(rec, httptest.NewRequest(http.MethodGet, "/ezviz/setup", nil))
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(body, `id="ezviz"`) || !strings.Contains(body, `id="sitepw"`) {
		t.Fatal("setup must wrap EZVIZ and site password in folds")
	}
	if !strings.Contains(body, `<details class="fold`) {
		t.Fatal("folds should be details.fold")
	}
	if strings.Contains(body, `id="ezviz" open`) {
		t.Fatal("configured EZVIZ account should start collapsed")
	}
	if strings.Contains(body, `id="sitepw" open`) {
		t.Fatal("site password should start collapsed")
	}
	if !strings.Contains(body, `autocomplete="new-password"`) {
		t.Fatal("password fields should not look like a login form to Safari")
	}
}

func TestSetupOpensEzvizFoldWhenUnconfigured(t *testing.T) {
	orig := streamer
	streamer = NewStreamer()
	t.Cleanup(func() { streamer = orig })
	origCfg := cfg
	t.Cleanup(func() {
		cfgMu.Lock()
		cfg = origCfg
		cfgMu.Unlock()
	})
	cfgMu.Lock()
	cfg.Email = ""
	cfg.Password = ""
	cfgMu.Unlock()

	rec := httptest.NewRecorder()
	handleSetup(rec, httptest.NewRequest(http.MethodGet, "/ezviz/setup", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `id="ezviz" open`) {
		t.Fatal("empty EZVIZ account should start open so first setup is visible")
	}
}
