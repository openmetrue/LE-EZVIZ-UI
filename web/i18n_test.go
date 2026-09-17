package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCatalogComplete(t *testing.T) {
	en := catalog["en"]
	if len(en) == 0 {
		t.Fatal("english catalog empty")
	}
	for _, l := range languages {
		m, ok := catalog[l.Code]
		if !ok {
			t.Errorf("missing catalog %s", l.Code)
			continue
		}
		for k := range en {
			if _, ok := m[k]; !ok {
				t.Errorf("%s missing key %s", l.Code, k)
			}
		}
	}
}

func TestLangOf(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/ezviz/login", nil)
	r.Header.Set("Accept-Language", "fr-FR,fr;q=0.9")
	if got := langOf(r); got != "fr" {
		t.Fatalf("accept-language: got %s", got)
	}

	r = httptest.NewRequest(http.MethodGet, "/ezviz/login", nil)
	r.AddCookie(&http.Cookie{Name: langCookie, Value: "ja"})
	r.Header.Set("Accept-Language", "ru")
	if got := langOf(r); got != "ja" {
		t.Fatalf("cookie should win: got %s", got)
	}

	if T("xx", "login.title") != T("en", "login.title") {
		t.Fatal("unknown lang should fall back to english")
	}
}
