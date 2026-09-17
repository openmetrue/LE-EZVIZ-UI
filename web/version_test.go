package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVersionsEqual(t *testing.T) {
	if !versionsEqual("v0.2.1", "0.2.1") {
		t.Fatal("v prefix should match")
	}
	if versionsEqual("v0.2.1", "v0.2.2") {
		t.Fatal("different versions")
	}
	if versionsEqual("dev", "v0.2.1") {
		t.Fatal("dev must not match a release")
	}
}

func TestInstallScriptURL(t *testing.T) {
	u := installScriptURL("openmetrue/LE-EZVIZ-UI", "v0.2.2")
	want := "https://github.com/openmetrue/LE-EZVIZ-UI/releases/download/v0.2.2/install.sh"
	if u != want {
		t.Fatalf("got %s", u)
	}
	latest := installScriptURL("openmetrue/LE-EZVIZ-UI", "latest")
	if !strings.Contains(latest, "/latest/download/install.sh") {
		t.Fatalf("got %s", latest)
	}
}

func TestLatestGitHubReleaseFromVERSIONAsset(t *testing.T) {
	orig := githubReleaseBase
	t.Cleanup(func() { githubReleaseBase = orig })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openmetrue/LE-EZVIZ-UI/releases/latest/download/VERSION" {
			t.Errorf("unexpected path %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Write([]byte("v0.5.39\n"))
	}))
	t.Cleanup(srv.Close)
	githubReleaseBase = srv.URL
	tag, err := latestGitHubRelease("openmetrue/LE-EZVIZ-UI")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.5.39" {
		t.Fatalf("tag=%s", tag)
	}
}

func TestLatestGitHubReleaseFromRedirect(t *testing.T) {
	orig := githubReleaseBase
	t.Cleanup(func() { githubReleaseBase = orig })
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/VERSION") {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path != "/openmetrue/LE-EZVIZ-UI/releases/latest" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Location", "/openmetrue/LE-EZVIZ-UI/releases/tag/v0.5.40")
		w.WriteHeader(http.StatusFound)
	}))
	t.Cleanup(srv.Close)
	githubReleaseBase = srv.URL
	tag, err := latestGitHubRelease("openmetrue/LE-EZVIZ-UI")
	if err != nil {
		t.Fatal(err)
	}
	if tag != "v0.5.40" {
		t.Fatalf("tag=%s", tag)
	}
}

func TestTagFromReleaseURL(t *testing.T) {
	if got := tagFromReleaseURL("https://github.com/openmetrue/LE-EZVIZ-UI/releases/tag/v0.5.39"); got != "v0.5.39" {
		t.Fatalf("got %s", got)
	}
	if got := tagFromReleaseURL("/releases/download/v0.5.39/VERSION"); got != "v0.5.39" {
		t.Fatalf("download path got %s", got)
	}
}
