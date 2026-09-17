package main

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type updateStatusFile struct {
	Phase  string `json:"phase"` // download|install|done|error
	Target string `json:"target"`
	Error  string `json:"error,omitempty"`
	At     string `json:"at"`
}

var (
	updateMu   sync.Mutex
	updateBusy bool
)

func updateStatusPath() string {
	return filepath.Join(*workDir, "update-status.json")
}

func writeUpdateStatus(phase, target, errStr string) {
	st := updateStatusFile{
		Phase:  phase,
		Target: target,
		Error:  errStr,
		At:     time.Now().UTC().Format(time.RFC3339),
	}
	data, err := json.Marshal(st)
	if err != nil {
		return
	}
	_ = os.WriteFile(updateStatusPath(), data, 0o644)
}

func readUpdateStatus() updateStatusFile {
	data, err := os.ReadFile(updateStatusPath())
	if err != nil {
		return updateStatusFile{}
	}
	var st updateStatusFile
	_ = json.Unmarshal(data, &st)
	return st
}

// startDetachedInstall runs install.sh in a transient systemd unit so that
// systemctl stop ezvizd does not kill the updater (cgroup isolation).
func startDetachedInstall(repo, tag string) error {
	if repo == "" {
		repo = defaultUpdateRepo
	}
	url := installScriptURL(repo, tag)
	status := updateStatusPath()
	unit := fmt.Sprintf("ezvizd-selfupdate-%d", time.Now().Unix())

	inner := fmt.Sprintf(`set -euo pipefail
STATUS=%q
TARGET=%q
URL=%q
REPO=%q
phase() {
  printf '{"phase":"%%s","target":"%%s","error":"%%s","at":"%%s"}\n' \
    "$1" "$TARGET" "$2" "$(date -u +%%Y-%%m-%%dT%%H:%%M:%%SZ)" > "$STATUS"
}
phase download ""
if ! curl -fsSL "$URL" | EZVIZ_VERSION="$TARGET" EZVIZ_REPO="$REPO" EZVIZ_UPDATE_STATUS="$STATUS" bash; then
  phase error "install.sh failed"
  exit 1
fi
phase done ""
`, status, tag, url, repo)

	cmd := exec.Command(
		"systemd-run",
		"--collect",
		"--no-block",
		"--unit="+unit,
		"--property=Type=oneshot",
		"/bin/bash", "-c", inner,
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemd-run: %w (%s)", err, strings.TrimSpace(string(out)))
	}
	log.Printf("update: started detached unit %s for %s", unit, tag)
	return nil
}

func scheduleInstall(from, tag string) {
	// Let the HTTP JSON response leave nginx before we spawn the updater.
	time.Sleep(500 * time.Millisecond)
	log.Printf("update: scheduling %s → %s", from, tag)
	writeUpdateStatus("download", tag, "")
	if err := startDetachedInstall(defaultUpdateRepo, tag); err != nil {
		log.Printf("update: detach failed: %v", err)
		writeUpdateStatus("error", tag, err.Error())
		updateMu.Lock()
		updateBusy = false
		updateMu.Unlock()
		return
	}
	// ezvizd may be stopped by install.sh; in-memory busy flag dies with us.
	// Progress continues via update-status.json on disk.
}
