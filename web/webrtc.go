package main

import (
	"encoding/json"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/pion/ice/v4"
	"github.com/pion/interceptor"
	"github.com/pion/rtcp"
	"github.com/pion/webrtc/v4"
	"github.com/pion/webrtc/v4/pkg/media"
	"github.com/pion/webrtc/v4/pkg/media/h264reader"
)

const h264Fmtp = "level-asymmetry-allowed=1;packetization-mode=1;profile-level-id=42e01f"

var (
	rtcAPI     *webrtc.API
	rtcMu      sync.Mutex
	rtcTrack   *webrtc.TrackLocalStaticSample
	rtcSPS     []byte
	rtcPPS     []byte
	rtcNeedIDR bool
)

func rtcEnabled() bool { return rtcAPI != nil }

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

func rtcMaybeCreateTrack() *webrtc.TrackLocalStaticSample {
	rtcMu.Lock()
	defer rtcMu.Unlock()
	if rtcTrack != nil {
		return rtcTrack
	}
	t, err := webrtc.NewTrackLocalStaticSample(
		webrtc.RTPCodecCapability{
			MimeType:     webrtc.MimeTypeH264,
			ClockRate:    90000,
			SDPFmtpLine:  h264Fmtp,
			RTCPFeedback: videoRTCPFeedback(),
		},
		"video", "ezvizd",
	)
	if err != nil {
		log.Printf("webrtc: track: %v", err)
		return nil
	}
	rtcTrack = t
	rtcNeedIDR = true
	log.Printf("webrtc: H264 track ready")
	return t
}

func rtcEnsureTrack() *webrtc.TrackLocalStaticSample {
	return rtcMaybeCreateTrack()
}

func rtcDropTrack() {
	rtcMu.Lock()
	rtcTrack = nil
	rtcSPS, rtcPPS = nil, nil
	rtcNeedIDR = true
	rtcMu.Unlock()
	ringClear()
}

func pumpH264(r io.Reader) {
	hr, err := h264reader.NewReader(r)
	if err != nil {
		log.Printf("webrtc: h264 reader: %v", err)
		return
	}
	logged := false
	for {
		nal, err := hr.NextNAL()
		if err != nil {
			return
		}
		rtcMu.Lock()
		switch nal.UnitType {
		case h264reader.NalUnitTypeSPS:
			rtcSPS = append([]byte(nil), nal.Data...)
		case h264reader.NalUnitTypePPS:
			rtcPPS = append([]byte(nil), nal.Data...)
		}
		rtcMu.Unlock()
		switch nal.UnitType {
		case h264reader.NalUnitTypeSPS, h264reader.NalUnitTypePPS:
			rtcMaybeCreateTrack()
			continue
		case h264reader.NalUnitTypeSEI, h264reader.NalUnitTypeAUD,
			h264reader.NalUnitTypeFiller, h264reader.NalUnitTypeEndOfSequence,
			h264reader.NalUnitTypeEndOfStream:
			continue
		}
		if nal.UnitType != h264reader.NalUnitTypeCodedSliceIdr &&
			nal.UnitType != h264reader.NalUnitTypeCodedSliceNonIdr &&
			nal.UnitType != h264reader.NalUnitTypeCodedSliceAux {
			continue
		}
		track := rtcMaybeCreateTrack()
		idr := nal.UnitType == h264reader.NalUnitTypeCodedSliceIdr
		rtcMu.Lock()
		needIDR := rtcNeedIDR
		sps, pps := rtcSPS, rtcPPS
		if idr {
			rtcNeedIDR = false
		}
		rtcMu.Unlock()
		if needIDR && !idr {
			continue
		}
		var au []byte
		if idr {
			au = annexBNAL(sps, pps, nal.Data)
		} else {
			au = annexBNAL(nal.Data)
		}
		ringPush(au, idr)
		if track != nil {
			if err := track.WriteSample(media.Sample{Data: au, Duration: time.Second / 15}); err != nil {
				if errors.Is(err, io.ErrClosedPipe) {
					continue
				}
				return
			}
		}
		if !logged {
			logged = true
			nt := nal.UnitType
			log.Printf("webrtc: first H264 sample type=%s bytes=%d", nt.String(), len(au))
		}
	}
}

func annexBNAL(nals ...[]byte) []byte {
	var out []byte
	start := []byte{0x00, 0x00, 0x00, 0x01}
	for _, n := range nals {
		if len(n) == 0 {
			continue
		}
		out = append(out, start...)
		out = append(out, n...)
	}
	return out
}

func offerH264Lines(sdp string) string {
	pts := map[string]bool{}
	var out []string
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "a=rtpmap:") && strings.Contains(strings.ToUpper(line), "H264") {
			out = append(out, line)
			fields := strings.Fields(line)
			if len(fields) >= 1 {
				pt := strings.TrimPrefix(fields[0], "a=rtpmap:")
				pt = strings.Split(pt, " ")[0]
				pts[pt] = true
			}
		}
	}
	for _, line := range strings.Split(sdp, "\n") {
		line = strings.TrimSpace(strings.TrimSuffix(line, "\r"))
		if !strings.HasPrefix(strings.ToLower(line), "a=fmtp:") {
			continue
		}
		rest := strings.TrimPrefix(line, "a=fmtp:")
		pt := strings.Fields(rest)[0]
		if pts[pt] {
			out = append(out, line)
		}
	}
	if len(out) > 4 {
		out = out[:4]
	}
	return strings.Join(out, " | ")
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
	log.Printf("webrtc: offer %s", offerH264Lines(offer.SDP))

	track := rtcEnsureTrack()
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
	go func() {
		for {
			pkts, _, err := sender.ReadRTCP()
			if err != nil {
				return
			}
			for _, p := range pkts {
				switch p.(type) {
				case *rtcp.PictureLossIndication, *rtcp.FullIntraRequest:
					if idr := ringLastIDR(); len(idr) > 0 {
						_ = track.WriteSample(media.Sample{Data: idr, Duration: time.Second / 15})
						_ = track.WriteSample(media.Sample{Data: idr, Duration: time.Second / 15})
					}
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
	rtcMu.Lock()
	rtcNeedIDR = true
	rtcMu.Unlock()
	loc := pc.LocalDescription()
	log.Printf("webrtc: answer %s", offerH264Lines(loc.SDP))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(loc)
}
