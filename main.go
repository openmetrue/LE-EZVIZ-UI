package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	client "le-ezviz-vs/client"
	logging "le-ezviz-vs/logging"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

var (
	email         = flag.String("email", "", "EZVIZ email (or EZVIZ_EMAIL)")
	password      = flag.String("password", "", "EZVIZ password (or EZVIZ_PASSWORD)")
	region        = flag.String("region", "Europe", "Europe|Africa|India|Oceania|NorthAmerica|Russia|SouthAmerica")
	deviceSerial  = flag.String("deviceSerial", "", "camera serial")
	out           = flag.String("out", "stream", "raw stream path, FIFO, or - for stdout")
	statusOnly    = flag.Bool("statusOnly", false, "print device status JSON and exit (no stream)")
	listDevices   = flag.Bool("listDevices", false, "print camera list JSON and exit (no stream)")
	maxStreamTime = flag.Int("maxStreamTime", 0, "reconnect VTDU every N seconds (battery cams)")
	idleWait      = flag.Bool("idleWait", false, "stay logged in; newline starts stream, SIGUSR1 stops it")
	logStdout     = flag.Bool("stdout", false, "write bridge log to stdout")
	logFile       = flag.Bool("logFile", false, "write bridge log to ./lez.log")
	log           *zap.Logger
)

func main() {
	flag.Parse()
	if *email == "" {
		*email = os.Getenv("EZVIZ_EMAIL")
	}
	if *password == "" {
		*password = os.Getenv("EZVIZ_PASSWORD")
	}
	if *email == "" || *password == "" {
		panic("email/password required")
	}
	logging.CreateLogger(*logFile, *logStdout)
	log = logging.Log
	if _, ok := client.Regions[*region]; !ok {
		log.Error("Invalid region")
	}

	lez, err := client.NewLE_EZVIZ_Client(*email, *password, *region, "00000000000000000000000000000000", "LE-EZ", "shipin7", 15)
	if err != nil {
		panic(err)
	}
	lez.LoadFeatureCode("featurecode")
	if _, err = lez.V3_Login(); err != nil {
		panic(err)
	}
	if *statusOnly {
		ds, err := lez.GetDeviceStatus(*deviceSerial)
		if err != nil {
			fmt.Fprintln(os.Stderr, "status:", err)
			os.Exit(1)
		}
		_ = json.NewEncoder(os.Stdout).Encode(ds)
		return
	}
	if _, err = lez.GetServerInfo(); err != nil {
		panic(err)
	}
	pageList, err := lez.GetPageList()
	if err != nil {
		panic(err)
	}
	if *listDevices {
		type listEntry struct {
			Serial  string `json:"serial"`
			Name    string `json:"name"`
			Channel int    `json:"channel"`
		}
		out := make([]listEntry, 0)
		if pageList.DeviceInfos != nil {
			for _, v := range *pageList.DeviceInfos {
				ch := v.ChannelNumber
				if ch == 0 {
					ch = 1
				}
				out = append(out, listEntry{Serial: v.DeviceSerial, Name: v.Name, Channel: ch})
			}
		}
		_ = json.NewEncoder(os.Stdout).Encode(out)
		return
	}
	for _, v := range *pageList.DeviceInfos {
		log.Info(v.Name, zap.String("Serial", v.DeviceSerial))
	}
	if *deviceSerial == "" {
		return
	}

	var ri client.Resource
	var di client.DeviceInfos
	for _, v := range *pageList.ResourceInfos {
		if v.DeviceSerial == *deviceSerial {
			ri = v
		}
	}
	for _, v := range *pageList.DeviceInfos {
		if v.DeviceSerial == *deviceSerial {
			di = v
		}
	}
	if di.ChannelNumber == 0 {
		di.ChannelNumber = 1
	}
	vtm, ok := pageList.VTM[ri.ResourceID]
	if !ok {
		panic("device VTM not found")
	}

	if *idleWait {
		runIdleWait(lez, vtm, ri, di)
		return
	}

	if err := openStreamOut(lez); err != nil {
		panic(err)
	}
	if err := startDeviceStream(lez, vtm, ri, di, nil); err != nil {
		log.Error("stream failed", zap.Error(err))
	}
}

func openStreamOut(lez *client.LE_EZVIZ_Client) error {
	if *out == "-" {
		lez.StreamOut = os.Stdout
		return nil
	}
	f, err := os.OpenFile(*out, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	lez.StreamOut = f
	return nil
}

func runIdleWait(lez *client.LE_EZVIZ_Client, v client.VTMResource, ri client.Resource, di client.DeviceInfos) {
	sigs := make(chan os.Signal, 1)
	notifyIdleStop(sigs)
	go func() {
		for range sigs {
			log.Info("idleWait: stop current stream")
			lez.InterruptStream()
		}
	}()
	warm := &warmVTM{}
	warm.prefetch(lez, v, ri, di)
	log.Info("EZVIZ session ready, waiting for stream kick")
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := in.ReadString('\n'); err != nil {
			return
		}
		log.Info("idleWait: starting stream")
		lez.BeginStream()
		if err := startDeviceStream(lez, v, ri, di, warm); err != nil {
			log.Error("stream session ended", zap.Error(err))
		} else {
			log.Info("stream session ended")
		}
		warm.prefetch(lez, v, ri, di)
	}
}

type warmVTM struct {
	mu  sync.Mutex
	gen uint64
	vs  *client.VTMStream
}

func (w *warmVTM) take() *client.VTMStream {
	w.mu.Lock()
	defer w.mu.Unlock()
	vs := w.vs
	w.vs = nil
	w.gen++
	return vs
}

func (w *warmVTM) prefetch(lez *client.LE_EZVIZ_Client, v client.VTMResource, ri client.Resource, di client.DeviceInfos) {
	w.mu.Lock()
	w.gen++
	gen := w.gen
	w.mu.Unlock()
	go func() {
		if _, err := lez.GetVTDUv2Token(); err != nil {
			log.Error("idleWait: prefetch token", zap.Error(err))
			return
		}
		vs, err := lez.DialVTM(v.ExternalIP, v.Port)
		if err != nil {
			log.Error("idleWait: prefetch VTM", zap.Error(err))
			return
		}
		vs.VTMPublicKey = v.PublicKey.Key
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.gen != gen {
			_ = vs.Conn.Close()
			return
		}
		if w.vs != nil && w.vs.Conn != nil {
			_ = w.vs.Conn.Close()
		}
		w.vs = vs
		log.Info("idleWait: warmed token + VTM")
	}()
}

func startDeviceStream(lez *client.LE_EZVIZ_Client, v client.VTMResource, ri client.Resource, di client.DeviceInfos, warm *warmVTM) error {
	defer lez.DropConns()
	var fifo *os.File
	if *idleWait && *out != "" && *out != "-" {
		defer func() {
			if fifo != nil {
				_ = fifo.Close()
				lez.StreamOut = nil
			}
		}()
	}
	for {
		t0 := time.Now()
		lez.DropConns()
		usedWarm := warm != nil
		fail := func(err error) error {
			if fifo == nil {
				unblockFifoWriter()
			}
			return err
		}
		vs, err := tokenAndVTM(lez, v, warm)
		if err != nil {
			return fail(err)
		}
		tokens := *lez.VTDUTokens.Tokens
		url := lez.BuildVtmUrl(vs.VTMIP, vs.VTMPort, ri.DeviceSerial, ri.StreamBizUrl, tokens[0], di.ChannelNumber, lez.ClientType)
		rsp, err := lez.StartVTMStream(vs, url)
		if err != nil {
			_ = vs.Conn.Close()
			if usedWarm {
				log.Warn("idleWait: warm VTM dead, redialing", zap.Error(err))
				warm = nil
				continue
			}
			return fail(err)
		}
		ip, port, _, _, err := lez.ParseVtmUrl(*rsp.Streamurl)
		if err != nil {
			_ = vs.Conn.Close()
			return fail(err)
		}
		vtdu, err := lez.ConnectVTDU(ip, port, *rsp.Vtmstreamkey, v.PublicKey.Key)
		if err != nil {
			_ = vs.Conn.Close()
			return fail(err)
		}
		if fifo == nil && *idleWait && *out != "" && *out != "-" {
			f, err := os.OpenFile(*out, os.O_WRONLY, 0)
			if err != nil {
				_ = vtdu.Conn.Close()
				_ = vs.Conn.Close()
				return err
			}
			fifo = f
			lez.StreamOut = f
		}
		log.Info("stream handshake", zap.Duration("total", time.Since(t0)))
		var proactive atomic.Bool
		var timer *time.Timer
		if *maxStreamTime > 0 {
			timer = time.AfterFunc(time.Duration(*maxStreamTime)*time.Second, func() {
				proactive.Store(true)
				log.Info("maxStreamTime reached, proactive VTDU reconnect")
				_ = vtdu.Conn.Close()
			})
		}
		err = lez.StartVTDUStream(vtdu, url)
		if timer != nil {
			timer.Stop()
		}
		_ = vtdu.Conn.Close()
		_ = vs.Conn.Close()
		if *idleWait && proactive.Load() && !lez.StreamInterrupted() {
			log.Info("idleWait: reconnecting VTDU")
			warm = nil
			continue
		}
		return err
	}
}

func tokenAndVTM(lez *client.LE_EZVIZ_Client, v client.VTMResource, warm *warmVTM) (*client.VTMStream, error) {
	var vs *client.VTMStream
	if warm != nil {
		vs = warm.take()
	}
	haveTok := lez.VTDUTokens != nil && lez.VTDUTokens.Tokens != nil && len(*lez.VTDUTokens.Tokens) > 0
	if vs != nil {
		lez.TrackConn(vs.Conn)
		if haveTok {
			return vs, nil
		}
		if _, err := lez.GetVTDUv2Token(); err != nil {
			_ = vs.Conn.Close()
			return nil, err
		}
		return vs, nil
	}
	tokCh := make(chan error, 1)
	go func() { _, err := lez.GetVTDUv2Token(); tokCh <- err }()
	dialed, err := lez.ConnectVTM(v.ExternalIP, v.Port, v.PublicKey.Key)
	tokErr := <-tokCh
	if err != nil {
		return nil, err
	}
	if tokErr != nil {
		_ = dialed.Conn.Close()
		return nil, tokErr
	}
	return dialed, nil
}
