package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func withTempWorkDir(t *testing.T) {
	t.Helper()
	orig := *workDir
	*workDir = t.TempDir()
	t.Cleanup(func() { *workDir = orig })
}

func withBasePath(t *testing.T, base string) {
	t.Helper()
	orig := *basePath
	*basePath = base
	t.Cleanup(func() { *basePath = orig })
}

func TestValidClipName(t *testing.T) {
	for _, n := range []string{
		"rec-2026-09-18_12-00-00.mp4",
		"rec-2026-09-18_12-00-00-2.mp4",
	} {
		if !validClipName(n) {
			t.Errorf("%s should be valid", n)
		}
	}
	for _, n := range []string{
		"", "clip.mp4", "rec-.mp4", "rec-2026-09-18_12-00-00.mov",
		"../rec-2026-09-18_12-00-00.mp4", "/etc/passwd",
		"rec-2026-09-18_12-00-00.mp4/../x",
	} {
		if validClipName(n) {
			t.Errorf("%s should be rejected", n)
		}
	}
}

func TestStoreClipRejectsBadName(t *testing.T) {
	withTempWorkDir(t)
	src := filepath.Join(t.TempDir(), "x")
	if err := os.WriteFile(src, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := storeClip(src, "../evil.mp4"); err == nil {
		t.Fatal("bad name must be rejected")
	}
}

func TestRecordingsStoreListServeDelete(t *testing.T) {
	withTempWorkDir(t)
	withBasePath(t, "/ezviz")

	body := "fake-mp4-bytes"
	src := filepath.Join(t.TempDir(), "src.mp4")
	if err := os.WriteFile(src, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	name := newClipName()
	info, err := storeClip(src, name)
	if err != nil {
		t.Fatal(err)
	}
	if info.Size != int64(len(body)) {
		t.Fatalf("size=%d", info.Size)
	}

	clips := listClips()
	if len(clips) != 1 || clips[0].Name != name {
		t.Fatalf("list=%v", clips)
	}

	rec := httptest.NewRecorder()
	handleRecordingFile(rec, httptest.NewRequest(http.MethodGet, "/ezviz/rec/"+name, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("serve status=%d", rec.Code)
	}
	if got := rec.Body.String(); got != body {
		t.Fatalf("body=%q", got)
	}

	rec = httptest.NewRecorder()
	handleRecordingFile(rec, httptest.NewRequest(http.MethodGet, "/ezviz/rec/"+name+"?dl=1", nil))
	if cd := rec.Header().Get("Content-Disposition"); !strings.Contains(cd, "attachment") {
		t.Fatalf("download disposition=%q", cd)
	}

	rec = httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/ezviz/recordings/delete", strings.NewReader("name="+name))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	handleRecordingDelete(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("delete status=%d", rec.Code)
	}
	if len(listClips()) != 0 {
		t.Fatal("clip should be gone")
	}
}

func TestRecordingFileRejectsTraversal(t *testing.T) {
	withTempWorkDir(t)
	withBasePath(t, "/ezviz")

	rec := httptest.NewRecorder()
	handleRecordingFile(rec, httptest.NewRequest(http.MethodGet, "/ezviz/rec/../config.json", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status=%d", rec.Code)
	}
}

func TestPruneClipsKeepsNewest(t *testing.T) {
	withTempWorkDir(t)
	if err := os.MkdirAll(recordingsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxClips+5; i++ {
		name := fmt.Sprintf("rec-2026-01-01_00-00-%03d.mp4", i)
		p := clipPath(name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := time.Unix(int64(i), 0)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	pruneClips()
	if got := len(listClips()); got != maxClips {
		t.Fatalf("kept %d want %d", got, maxClips)
	}
}

func TestRecordingsAPIEmpty(t *testing.T) {
	withTempWorkDir(t)
	rec := httptest.NewRecorder()
	handleRecordingsAPI(rec, httptest.NewRequest(http.MethodGet, "/ezviz/api/recordings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), `"clips":[]`) {
		t.Fatalf("body=%s", rec.Body.String())
	}
}

func TestRecordingsPageShowsClip(t *testing.T) {
	withTempWorkDir(t)
	withBasePath(t, "/ezviz")
	if err := os.MkdirAll(recordingsDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	name := "rec-2026-09-18_12-00-00.mp4"
	if err := os.WriteFile(clipPath(name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleRecordings(rec, httptest.NewRequest(http.MethodGet, "/ezviz/recordings", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{
		name, "/ezviz/rec/" + name, "?dl=1", "/ezviz/recordings/delete", `id="rv"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("page missing %q", want)
		}
	}
}
