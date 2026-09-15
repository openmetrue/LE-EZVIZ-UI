// ezvizd is the LE-EZVIZ-UI daemon: site password, on-demand WebRTC,
// recordings and battery history on top of the le-ezviz-vs bridge.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"
)

var (
	listenAddr = flag.String("listen", "127.0.0.1:8090", "listen address")
	configPath = flag.String("config", "/opt/ezvizd/config.json", "path to config.json")
	workDir    = flag.String("workdir", "/var/lib/ezvizd", "working directory (logs, recordings)")
	bridgePath = flag.String("bridge", "/opt/ezvizd/le-ezviz-vs", "path to the le-ezviz-vs bridge")
	ffmpegPath = flag.String("ffmpeg", "/usr/bin/ffmpeg", "path to ffmpeg")
	basePath   = flag.String("base", "/ezviz", "URL prefix behind nginx")
	idleSec    = flag.Int("idle", 10, "seconds without viewers before the stream stops")
	webrtcUDP  = flag.Int("webrtc-udp", 8091, "UDP port for WebRTC ICE; 0 disables")
	webrtcIP   = flag.String("webrtc-ip", "", "public IPv4 advertised in ICE (empty = auto)")
	streamer   *Streamer
)

func main() {
	flag.Parse()
	idleTimeout = time.Duration(*idleSec) * time.Second
	if err := loadConfig(); err != nil {
		log.Fatalf("config: %v", err)
	}
	if err := os.MkdirAll(recDir(), 0o755); err != nil {
		log.Fatalf("recdir: %v", err)
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
	if err := initWebRTC(); err != nil {
		log.Printf("webrtc: %v (Live unavailable)", err)
	}

	b := *basePath
	mux := http.NewServeMux()
	mux.HandleFunc(b+"/init", handleInit)
	mux.HandleFunc(b+"/login", handleLogin)
	mux.HandleFunc(b+"/lang", handleLang)
	mux.HandleFunc(b+"/logout", handleLogout)
	mux.HandleFunc(b+"/setup", auth(handleSetup))
	mux.HandleFunc(b+"/", auth(handlePlayer))
	mux.HandleFunc(b+"/share", authOrToken(handleShare))
	mux.HandleFunc(b+"/start", authOrToken(handleStart))
	mux.HandleFunc(b+"/webrtc", authOrToken(handleWebRTC))
	mux.HandleFunc(b+"/save", auth(handleSave))
	mux.HandleFunc(b+"/recordings", auth(handleRecordings))
	mux.HandleFunc(b+"/rec/", auth(handleRecFile))
	mux.HandleFunc(b+"/api/status", authOrToken(handleStatus))
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
