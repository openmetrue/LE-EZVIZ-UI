package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Version is set at release build via -ldflags "-X main.Version=vX.Y.Z".
var Version = "dev"

const defaultUpdateRepo = "openmetrue/LE-EZVIZ-UI"

func normalizeVersion(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return strings.TrimPrefix(v, "v")
}

func versionsEqual(a, b string) bool {
	na, nb := normalizeVersion(a), normalizeVersion(b)
	if na == "" || nb == "" || na == "dev" || nb == "dev" {
		return false
	}
	return strings.EqualFold(na, nb)
}

func installedVersion() string {
	if v := strings.TrimSpace(Version); v != "" && v != "dev" {
		return v
	}
	for _, p := range []string{
		filepath.Join(filepath.Dir(*configPath), "VERSION"),
		"/opt/ezvizd/VERSION",
	} {
		b, err := os.ReadFile(p)
		if err == nil {
			if s := strings.TrimSpace(string(b)); s != "" {
				return s
			}
		}
	}
	return "dev"
}

// githubReleaseBase is the public site (not api.github.com). VPS IPs often
// get HTTP 403 from the unauthenticated REST API.
var githubReleaseBase = "https://github.com"

func latestGitHubRelease(repo string) (string, error) {
	if repo == "" {
		repo = defaultUpdateRepo
	}
	if tag, err := latestReleaseFromVERSION(repo); err == nil {
		return tag, nil
	}
	if tag, err := latestReleaseFromRedirect(repo); err == nil {
		return tag, nil
	}
	return latestReleaseFromAPI(repo)
}

func githubUserAgent() string {
	return "ezvizd/" + installedVersion()
}

func githubDo(client *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", githubUserAgent())
	return client.Do(req)
}

func latestReleaseFromVERSION(repo string) (string, error) {
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := githubDo(client, githubReleaseBase+"/"+repo+"/releases/latest/download/VERSION")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 4096))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github VERSION: HTTP %d", res.StatusCode)
	}
	return normalizeReleaseTag(string(body))
}

func latestReleaseFromRedirect(repo string) (string, error) {
	client := &http.Client{
		Timeout: 20 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	res, err := githubDo(client, githubReleaseBase+"/"+repo+"/releases/latest")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusFound && res.StatusCode != http.StatusMovedPermanently &&
		res.StatusCode != http.StatusTemporaryRedirect && res.StatusCode != http.StatusPermanentRedirect &&
		res.StatusCode != http.StatusSeeOther {
		return "", fmt.Errorf("github latest: HTTP %d", res.StatusCode)
	}
	tag := tagFromReleaseURL(res.Header.Get("Location"))
	if tag == "" {
		return "", fmt.Errorf("github latest: no tag in Location")
	}
	return tag, nil
}

func latestReleaseFromAPI(repo string) (string, error) {
	req, err := http.NewRequest(http.MethodGet, "https://api.github.com/repos/"+repo+"/releases/latest", nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", githubUserAgent())
	client := &http.Client{Timeout: 20 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return "", err
	}
	if res.StatusCode != http.StatusOK {
		return "", fmt.Errorf("github: HTTP %d", res.StatusCode)
	}
	var payload struct {
		TagName string `json:"tag_name"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", err
	}
	return normalizeReleaseTag(payload.TagName)
}

func normalizeReleaseTag(s string) (string, error) {
	tag := strings.TrimSpace(s)
	if tag == "" {
		return "", fmt.Errorf("github: empty tag")
	}
	if !strings.HasPrefix(tag, "v") {
		tag = "v" + tag
	}
	return tag, nil
}

func tagFromReleaseURL(u string) string {
	u = strings.TrimSpace(u)
	for _, needle := range []string{"/releases/tag/", "/releases/download/"} {
		i := strings.Index(u, needle)
		if i < 0 {
			continue
		}
		tag := u[i+len(needle):]
		if j := strings.IndexAny(tag, "/?#"); j >= 0 {
			tag = tag[:j]
		}
		tag = strings.TrimSpace(tag)
		if tag != "" {
			return tag
		}
	}
	return ""
}

func installScriptURL(repo, tag string) string {
	if repo == "" {
		repo = defaultUpdateRepo
	}
	if tag == "" || tag == "latest" {
		return "https://github.com/" + repo + "/releases/latest/download/install.sh"
	}
	return "https://github.com/" + repo + "/releases/download/" + tag + "/install.sh"
}
