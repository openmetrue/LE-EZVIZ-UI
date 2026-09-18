// ezvizd is the LE-EZVIZ-UI daemon: site password, on-demand Live,
// and battery history on top of the le-ezviz-vs bridge.
package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	listenAddr = flag.String("listen", "127.0.0.1:8090", "listen address")
	configPath = flag.String("config", "/opt/ezvizd/config.json", "path to config.json")
	workDir    = flag.String("workdir", "/var/lib/ezvizd", "working directory (logs, stats)")
	bridgePath = flag.String("bridge", "/opt/ezvizd/le-ezviz-vs", "path to the le-ezviz-vs bridge")
	ffmpegPath = flag.String("ffmpeg", "/usr/bin/ffmpeg", "path to ffmpeg")
	basePath   = flag.String("base", "/ezviz", "URL prefix behind nginx")
	idleSec    = flag.Int("idle", 10, "seconds without viewers before the stream stops")
	webrtcUDP  = flag.Int("webrtc-udp", 8091, "unused; kept so existing systemd units still start")
	webrtcIP   = flag.String("webrtc-ip", "", "unused; kept so existing systemd units still start")
	streamer   *Streamer
)

func main() {
	versionFlag := flag.Bool("version", false, "print version and exit")
	flag.Parse()
	if *versionFlag {
		fmt.Println(installedVersion())
		return
	}
	idleTimeout = time.Duration(*idleSec) * time.Second
	if err := loadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(*workDir, 0o755); err != nil {
		log.Fatalf("workdir: %v", err)
	}
	streamer = NewStreamer()
	devStatusLoad()
	statsLoad()
	go streamer.supervise()
	if streamer.configured() {
		email, password, serial, region := streamer.creds()
		go func() {
			if err := streamer.ensureHold(email, password, serial, region); err != nil {
				log.Printf("streamer: session: %v", err)
			}
		}()
	}
	go statsCollector()
	go logRotator()
	reconcileUpdateStatusOnBoot()

	b := *basePath
	mux := http.NewServeMux()
	mux.HandleFunc(b+"/init", handleInit)
	mux.HandleFunc(b+"/login", handleLogin)
	mux.HandleFunc(b+"/lang", handleLang)
	mux.HandleFunc(b+"/logout", handleLogout)
	mux.HandleFunc(b+"/setup", auth(handleSetup))
	mux.HandleFunc(b+"/", auth(handlePlayer))
	mux.Handle(b+"/static/", http.StripPrefix(b+"/static/", staticHandler()))
	mux.HandleFunc(b+"/live", auth(handleLive))
	mux.HandleFunc(b+"/save", auth(handleSave))
	mux.HandleFunc(b+"/recordings", auth(handleRecordings))
	mux.HandleFunc(b+"/recordings/delete", auth(handleRecordingDelete))
	mux.HandleFunc(b+"/rec/", auth(handleRecordingFile))
	mux.HandleFunc(b+"/api/recordings", auth(handleRecordingsAPI))
	mux.HandleFunc(b+"/api/status", auth(handleStatus))
	mux.HandleFunc(b+"/api/version", auth(handleVersionAPI))
	mux.HandleFunc(b+"/api/update", auth(handleUpdateAPI))
	mux.HandleFunc(b+"/stats", auth(handleStats))
	mux.HandleFunc(b+"/api/stats", auth(handleStatsAPI))
	mux.HandleFunc(b+"/api/devices", auth(handleDevicesAPI))
	mux.HandleFunc(b+"/api/live/device", auth(handleLiveDevice))
	mux.HandleFunc(b+"/maint", auth(handleMaint))
	mux.HandleFunc(b+"/logs", auth(handleLogs))

	log.Printf("ezvizd listening on %s (base %s) live=fmp4", *listenAddr, b)
	log.Fatal(http.ListenAndServe(*listenAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		mux.ServeHTTP(w, r)
	})))
}
