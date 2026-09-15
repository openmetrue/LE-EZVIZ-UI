package main

import (
	"bufio"
	"encoding/hex"
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

var email = flag.String("email", "", "EZVIZ user e-mail")
var password = flag.String("password", "", "EZVIZ user password")
var region = flag.String("region", "Europe", "Manually set the region you are in: Europe, Africa, India, Oceania, NorthAmercia, Russia, SouthAmerica")
var terminalName = flag.String("terminalName", "LE-EZ", "Optional: Set the name of what your device is called when viewing registered devices (terminals)")
var preservefc = flag.Bool("preserveFeatureCode", true, "Preserve the featurecode, it is essentially a random ID to identify the terminal")
var deviceSerial = flag.String("deviceSerial", "", "The device serial you want to connect to")
var out = flag.String("out", "stream", "Where to write the raw stream: file path or - for stdout (pipe mode, logs go to lez.log)")
var statusOnly = flag.Bool("statusOnly", false, "Print device status as JSON to stdout and exit (no stream, camera stays asleep)")
var maxStreamTime = flag.Int("maxStreamTime", 0, "If >0, proactively reconnect the VTDU stream every N seconds (keeps battery cameras awake past their KeepAlive limit)")
var idleWait = flag.Bool("idleWait", false, "Stay logged in; wait for a newline on stdin before each stream, SIGUSR1 ends the current stream")
var stdout = flag.Bool("stdout", true, "Print log to the terminal")
var logFile = flag.Bool("logFile", true, "Print log to lez.log")
var logLevel = flag.String("logLevel", "info", "Log level: debug, info, warn, error")
var log *zap.Logger

func main() {
	flag.Parse()
	// Credentials may come from env to avoid showing them in the process list.
	if *email == "" {
		*email = os.Getenv("EZVIZ_EMAIL")
	}
	if *password == "" {
		*password = os.Getenv("EZVIZ_PASSWORD")
	}
	if *email == "" {
		panic("email empty")
	}
	if *password == "" {
		panic("password empty")
	}
	if *out == "-" || *statusOnly || *idleWait {
		*stdout = false
		*logFile = true
	}
	logging.CreateLogger(*logFile, *stdout, *logLevel)
	client.SetLogger(logging.Log)
	client.TerminalName = *terminalName
	log = logging.Log
	if _, ok := client.Regions[*region]; !ok {
		log.Error("Invalid region", zap.String("Valid Values", "Europe|Africa|India|Oceania|NorthAmerica|Russia|SouthAmerica"))
	}
	LEZ, err := client.NewLE_EZVIZ_Client(*email, *password, *region, "00000000000000000000000000000000", *terminalName, "shipin7", 15)
	if err != nil {
		panic(err)
	}
	if *out == "-" {
		LEZ.PipeMode = true
		LEZ.StreamOut = os.Stdout
	} else if *out != "" {
		LEZ.StreamFile = *out
	}
	if *preservefc {
		LEZ.LoadFeatureCode("featurecode")
	} else {
		LEZ.SetFeatureCode(hex.EncodeToString(client.GenerateFeatureCode()))
	}
	if _, err = LEZ.V3_Login(); err != nil {
		panic(err)
	}
	if *statusOnly {
		ds, err := LEZ.GetDeviceStatus(*deviceSerial)
		if err != nil {
			fmt.Fprintln(os.Stderr, "status query failed:", err)
			os.Exit(1)
		}
		json.NewEncoder(os.Stdout).Encode(ds)
		return
	}
	if _, err = LEZ.GetServerInfo(); err != nil {
		panic(err)
	}

	PageList, err := LEZ.GetPageList()
	if err != nil {
		panic(err)
	}
	if *deviceSerial == "" {
		if _, err = LEZ.GetVTDUv2Token(); err != nil {
			panic(err)
		}
	}
	for _, v := range *PageList.DeviceInfos {
		log.Info(v.Name, zap.String("Serial", v.DeviceSerial))
	}
	fmt.Fprintln(os.Stderr, "!!!WARNING: This library is in beta, only use for development/testing until it is stable, things will change as development continues!!!")
	fmt.Fprintln(os.Stderr, "!!!Encryption is not yet available including E2EE with stream servers, your streams will be unencrypted until encryption is implemented!!!")
	if *deviceSerial != "" {
		var RI client.Resource
		var DI client.DeviceInfos
		for _, v := range *PageList.ResourceInfos {
			if *deviceSerial == v.DeviceSerial {
				RI = v
			}
		}
		for _, v := range *PageList.DeviceInfos {
			if *deviceSerial == v.DeviceSerial {
				DI = v
			}
		}
		if DI.ChannelNumber == 0 {
			DI.ChannelNumber = 1
		}
		if v, ok := PageList.VTM[RI.ResourceID]; ok {
			run := func() error {
				return startDeviceStream(LEZ, v, RI, DI, nil)
			}
			if *idleWait {
				runIdleWait(LEZ, v, RI, DI)
				return
			}
			if !LEZ.PipeMode {
				if err := run(); err != nil {
					log.Error("stream failed", zap.Error(err))
				}
				return
			}
			backoff := time.Duration(0)
			for {
				if backoff > 0 {
					time.Sleep(backoff)
				}
				iterStart := time.Now()
				err := run()
				if err != nil {
					log.Error("VTDU stream ended with error", zap.Error(err))
				} else {
					log.Info("VTDU stream ended, reconnecting")
				}
				if time.Since(iterStart) < 30*time.Second {
					backoff = 15 * time.Second
				} else {
					backoff = 2 * time.Second
				}
			}
		}
	}

}

func runIdleWait(LEZ *client.LE_EZVIZ_Client, v client.VTMResource, RI client.Resource, DI client.DeviceInfos) {
	sigs := make(chan os.Signal, 1)
	notifyIdleStop(sigs)
	go func() {
		for range sigs {
			log.Info("idleWait: stop current stream")
			LEZ.InterruptStream()
		}
	}()
	warm := &warmVTM{}
	warm.prefetch(LEZ, v, RI, DI)
	log.Info("EZVIZ session ready, waiting for stream kick")
	in := bufio.NewReader(os.Stdin)
	for {
		if _, err := in.ReadString('\n'); err != nil {
			return
		}
		log.Info("idleWait: starting stream")
		LEZ.BeginStream()
		if err := startDeviceStream(LEZ, v, RI, DI, warm); err != nil {
			log.Error("stream session ended", zap.Error(err))
		} else {
			log.Info("stream session ended")
		}
		warm.prefetch(LEZ, v, RI, DI)
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

func (w *warmVTM) prefetch(LEZ *client.LE_EZVIZ_Client, v client.VTMResource, RI client.Resource, DI client.DeviceInfos) {
	w.mu.Lock()
	w.gen++
	gen := w.gen
	w.mu.Unlock()
	go func() {
		if _, err := LEZ.GetVTDUv2Token(); err != nil {
			log.Error("idleWait: prefetch token", zap.Error(err))
			return
		}
		vs, err := LEZ.DialVTM(v.ExternalIP, v.Port)
		if err != nil {
			log.Error("idleWait: prefetch VTM", zap.Error(err))
			return
		}
		vs.VTMPublicKey = v.PublicKey.Key
		w.mu.Lock()
		defer w.mu.Unlock()
		if w.gen != gen {
			vs.Conn.Close()
			return
		}
		if w.vs != nil && w.vs.Conn != nil {
			_ = w.vs.Conn.Close()
		}
		w.vs = vs
		log.Info("idleWait: warmed token + VTM")
	}()
}

func startDeviceStream(LEZ *client.LE_EZVIZ_Client, v client.VTMResource, RI client.Resource, DI client.DeviceInfos, warm *warmVTM) error {
	defer LEZ.DropConns()
	var fifo *os.File
	if *idleWait && *out != "" && *out != "-" {
		defer func() {
			if fifo != nil {
				fifo.Close()
				LEZ.StreamOut = nil
			}
		}()
	}
	for {
		t0 := time.Now()
		LEZ.DropConns()
		usedWarm := warm != nil
		fail := func(err error) error {
			if fifo == nil {
				unblockFifoWriter()
			}
			return err
		}
		VS, err := tokenAndVTM(LEZ, v, RI, DI, warm)
		if err != nil {
			return fail(err)
		}
		Tokens := *LEZ.VTDUTokens.Tokens
		URL := LEZ.BuildVtmUrl(VS.VTMIP, VS.VTMPort, RI.DeviceSerial, RI.StreamBizUrl, Tokens[0], DI.ChannelNumber, LEZ.ClientType)
		RStreamInfoRsp, err := LEZ.StartVTMStream(VS, URL)
		if err != nil {
			VS.Conn.Close()
			if usedWarm {
				log.Warn("idleWait: warm VTM dead, redialing", zap.Error(err))
				warm = nil
				continue
			}
			return fail(err)
		}
		vtmAt := time.Since(t0)
		IP, Port, _, _, err := LEZ.ParseVtmUrl(*RStreamInfoRsp.Streamurl)
		if err != nil {
			VS.Conn.Close()
			return fail(err)
		}
		VTDUStream, err := LEZ.ConnectVTDU(IP, Port, *RStreamInfoRsp.Vtmstreamkey, v.PublicKey.Key)
		if err != nil {
			VS.Conn.Close()
			return fail(err)
		}
		if fifo == nil && *idleWait && *out != "" && *out != "-" {
			f, err := os.OpenFile(*out, os.O_WRONLY, 0)
			if err != nil {
				VTDUStream.Conn.Close()
				VS.Conn.Close()
				return err
			}
			fifo = f
			LEZ.PipeMode = true
			LEZ.StreamOut = f
		}
		log.Info("stream handshake",
			zap.Duration("vtm", vtmAt),
			zap.Duration("vtdu", time.Since(t0)-vtmAt),
			zap.Duration("total", time.Since(t0)))
		var proactive atomic.Bool
		var timer *time.Timer
		if *maxStreamTime > 0 {
			timer = time.AfterFunc(time.Duration(*maxStreamTime)*time.Second, func() {
				proactive.Store(true)
				log.Info("maxStreamTime reached, proactive VTDU reconnect")
				VTDUStream.Conn.Close()
			})
		}
		err = LEZ.StartVTDUStream(VTDUStream, URL)
		if timer != nil {
			timer.Stop()
		}
		VTDUStream.Conn.Close()
		VS.Conn.Close()
		if *idleWait && proactive.Load() && !LEZ.StreamInterrupted() {
			log.Info("idleWait: reconnecting VTDU")
			warm = nil
			continue
		}
		return err
	}
}

func tokenAndVTM(LEZ *client.LE_EZVIZ_Client, v client.VTMResource, RI client.Resource, DI client.DeviceInfos, warm *warmVTM) (*client.VTMStream, error) {
	var vs *client.VTMStream
	if warm != nil {
		vs = warm.take()
	}
	haveTok := LEZ.VTDUTokens != nil && LEZ.VTDUTokens.Tokens != nil && len(*LEZ.VTDUTokens.Tokens) > 0
	if vs != nil {
		LEZ.TrackConn(vs.Conn)
		if haveTok {
			return vs, nil
		}
		if _, err := LEZ.GetVTDUv2Token(); err != nil {
			vs.Conn.Close()
			return nil, err
		}
		return vs, nil
	}
	tokCh := make(chan error, 1)
	go func() { _, err := LEZ.GetVTDUv2Token(); tokCh <- err }()
	dialed, err := LEZ.ConnectVTM(v.ExternalIP, v.Port, RI, DI, "", v.PublicKey.Key)
	tokErr := <-tokCh
	if err != nil {
		return nil, err
	}
	if tokErr != nil {
		dialed.Conn.Close()
		return nil, tokErr
	}
	return dialed, nil
}
