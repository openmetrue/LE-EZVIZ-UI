package main

import (
	"encoding/json"
	"log"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
)

const h264Fmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

var rtcAPI *webrtc.API

func publicIPv4() string {
	if *webrtcIP != "" {
		return *webrtcIP
	}
	ifaces, err := net.Interfaces()
	if err != nil {
		return ""
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip := ipn.IP.To4()
			if ip == nil || ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsPrivate() {
				continue
			}
			return ip.String()
		}
	}
	return ""
}

func videoRTCPFeedback() []webrtc.RTCPFeedback {
	return []webrtc.RTCPFeedback{
		{Type: "goog-remb", Parameter: ""},
		{Type: "ccm", Parameter: "fir"},
		{Type: "nack", Parameter: ""},
		{Type: "nack", Parameter: "pli"},
	}
}

func initWebRTC() error {
	if *webrtcUDP <= 0 {
		log.Printf("webrtc: disabled")
		return nil
	}
	mux, err := ice.NewMultiUDPMuxFromPort(*webrtcUDP,
		ice.UDPMuxFromPortWithNetworks(ice.NetworkTypeUDP4),
		ice.UDPMuxFromPortWithIPFilter(func(ip net.IP) bool {
			return ip.To4() != nil && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
		}),
	)
	if err != nil {
		return err
	}
	se := webrtc.SettingEngine{}
	se.SetICEUDPMux(mux)
	if ip := publicIPv4(); ip != "" {
		se.SetNAT1To1IPs([]string{ip}, webrtc.ICECandidateTypeHost)
		log.Printf("webrtc: UDP %d ICE %s (H264)", *webrtcUDP, ip)
	} else {
		log.Printf("webrtc: UDP %d (no public IPv4 found, ICE may fail)", *webrtcUDP)
	}

	m := &webrtc.MediaEngine{}
	// Constrained Baseline — Safari WebRTC plays this reliably; 1080p HEVC does not.
	if err := m.RegisterCodec(webrtc.RTPCodecParameters{
		RTPCodecCapability: webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeH264,
			ClockRate:    90000,
			SDPFmtpLine:  h264Fmtp,
			RTCPFeedback: videoRTCPFeedback(),
		},
		PayloadType: 96,
	}, webrtc.RTPCodecTypeVideo); err != nil {
		return err
	}
	ir := &interceptor.Registry{}
	if err := webrtc.RegisterDefaultInterceptors(m, ir); err != nil {
		return err
	}
	rtcAPI = webrtc.NewAPI(
		webrtc.WithSettingEngine(se),
		webrtc.WithMediaEngine(m),
		webrtc.WithInterceptorRegistry(ir),
	)
	return nil
}

func handleWebRTC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if rtcAPI == nil {
		http.Error(w, "webrtc disabled", http.StatusServiceUnavailable)
		return
	}
	var offer webrtc.SessionDescription
	if err := json.NewDecoder(r.Body).Decode(&offer); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if !strings.Contains(strings.ToUpper(offer.SDP), "H264") {
		http.Error(w, "offer has no H264", http.StatusBadRequest)
		return
	}
	log.Printf("webrtc: offer ok")

	// Arm keyframe gating BEFORE AddTrack so the new subscriber cannot
	// receive mid-GOP P-frames (Safari then stays gray forever).
	track, seed := live.BeginViewer()
	if track == nil {
		http.Error(w, "no track", http.StatusServiceUnavailable)
		return
	}

	pc, err := rtcAPI.NewPeerConnection(webrtc.Configuration{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	sender, err := pc.AddTrack(track)
	if err != nil {
		pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	live.SeedViewer(track, seed)

	go func() {
		for {
			pkts, _, err := sender.ReadRTCP()
			if err != nil {
				return
			}
			for _, p := range pkts {
				switch p.(type) {
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					live.OnPLI(track)
				}
			}
		}
	}()
	pc.OnConnectionStateChange(func(st webrtc.PeerConnectionState) {
		log.Printf("webrtc: peer %s", st)
		if st == webrtc.PeerConnectionStateFailed || st == webrtc.PeerConnectionStateClosed {
			_ = pc.Close()
		}
	})
	if err := pc.SetRemoteDescription(offer); err != nil {
		pc.Close()
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gather := webrtc.GatheringCompletePromise(pc)
	if err := pc.SetLocalDescription(answer); err != nil {
		pc.Close()
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	select {
	case <-gather:
	case <-time.After(100 * time.Millisecond):
	}
	// Keep needKey armed until the next encoder IDR after this join.
	live.RequestKey()
	loc := pc.LocalDescription()
	log.Printf("webrtc: answer ok")
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loc)
}
