// ezvizd is the LE-EZVIZ-UI daemon: site password, on-demand HLS,
// recordings and battery history on top of the le-ezviz-vs bridge.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

var (
	listenAddr = flag.String("listen", "127.0.0.1:8090", "listen address")
	configPath = flag.String("config", "/opt/ezvizd/config.json", "path to config.json")
	workDir    = flag.String("workdir", "/var/lib/ezvizd", "working directory (hls/, logs)")
	bridgePath = flag.String("bridge", "/opt/ezvizd/le-ezviz-vs", "path to the le-ezviz-vs bridge")
	ffmpegPath = flag.String("ffmpeg", "/usr/bin/ffmpeg", "path to ffmpeg")
	basePath   = flag.String("base", "/ezviz", "URL prefix behind nginx")
	idleSec    = flag.Int("idle", 30, "seconds without viewers before the stream stops")
	streamer   *Streamer
)

func main() {
	flag.Parse()
	idleTimeout = time.Duration(*idleSec) * time.Second
	if err := loadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(*workDir, "hls"), 0o755); err != nil {
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

	b := *basePath
	mux := http.NewServeMux()
	mux.HandleFunc(b+"/init", handleInit)
	mux.HandleFunc(b+"/login", handleLogin)
	mux.HandleFunc(b+"/lang", handleLang)
	mux.HandleFunc(b+"/logout", handleLogout)
	mux.HandleFunc(b+"/setup", auth(handleSetup))
	mux.HandleFunc(b+"/", auth(handlePlayer))
	mux.HandleFunc(b+"/start", auth(handleStart))
	mux.HandleFunc(b+"/preview.jpg", authOrToken(handlePreview))
	mux.HandleFunc(b+"/hls/", authOrToken(handleHLS))
	mux.HandleFunc(b+"/save", auth(handleSave))
	mux.HandleFunc(b+"/recordings", auth(handleRecordings))
	mux.HandleFunc(b+"/rec/", auth(handleRecFile))
	mux.HandleFunc(b+"/api/status", auth(handleStatus))
	mux.HandleFunc(b+"/stats", auth(handleStats))
	mux.HandleFunc(b+"/api/stats", auth(handleStatsAPI))
	mux.HandleFunc(b+"/maint", auth(handleMaint))
	mux.HandleFunc(b+"/logs", auth(handleLogs))

	log.Printf("ezvizd listening on %s (base %s)", *listenAddr, b)
	log.Fatal(http.ListenAndServe(*listenAddr, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		securityHeaders(w)
		mux.ServeHTTP(w, r)
	})))
}
