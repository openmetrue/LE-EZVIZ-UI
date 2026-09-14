package main

import (
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	client "le-ezviz-vs/client"
	logging "le-ezviz-vs/logging"
	"os"
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
var statusRaw = flag.Bool("statusRaw", false, "Print raw STATUS,WIFI pagelist JSON to stdout and exit (debug)")
var preserveSession = flag.Bool("preserveSession", false, "Preserve session token") //TBD
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
	if *out == "-" || *statusOnly || *statusRaw {
		*stdout = false
		*logFile = true
	}
	logging.CreateLogger(*logFile, *stdout, *logLevel)
	client.SetLogger(logging.Log)
	client.CurrentRegion = *region
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
	if *statusRaw {
		raw, err := LEZ.GetStatusRaw()
		if err != nil {
			fmt.Fprintln(os.Stderr, "status query failed:", err)
			os.Exit(1)
		}
		fmt.Println(raw)
		return
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
	_, err = LEZ.GetServerInfo()
	if err != nil {
		panic(err)
	}

	PageList, err := LEZ.GetPageList()
	if err != nil {
		panic(err)
	}
	_, err = LEZ.GetVTDUv2Token()
	if err != nil {
		panic(err)
	}
	for _, v := range *PageList.DeviceInfos {
		log.Info(v.Name, zap.String("Serial", v.DeviceSerial))
	}
	// fmt.Println(PageList.VTM)
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
				return startDeviceStream(LEZ, v, RI, DI)
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

func startDeviceStream(LEZ *client.LE_EZVIZ_Client, v client.VTMResource, RI client.Resource, DI client.DeviceInfos) error {
	if _, err := LEZ.GetVTDUv2Token(); err != nil {
		return err
	}
	VS, err := LEZ.ConnectVTM(v.ExternalIP, v.Port, RI, DI, "", v.PublicKey.Key)
	if err != nil {
		return err
	}
	defer VS.Conn.Close()
	Tokens := *LEZ.VTDUTokens.Tokens
	URL := LEZ.BuildVtmUrl(VS.VTMIP, VS.VTMPort, RI.DeviceSerial, RI.StreamBizUrl, Tokens[0], DI.ChannelNumber, LEZ.ClientType)
	RStreamInfoRsp, err := LEZ.StartVTMStream(VS, URL)
	if err != nil {
		return err
	}
	IP, Port, _, _, err := LEZ.ParseVtmUrl(*RStreamInfoRsp.Streamurl)
	if err != nil {
		return err
	}
	VTDUStream, err := LEZ.ConnectVTDU(IP, Port, *RStreamInfoRsp.Vtmstreamkey, v.PublicKey.Key)
	if err != nil {
		return err
	}
	defer VTDUStream.Conn.Close()
	if *maxStreamTime > 0 {
		t := time.AfterFunc(time.Duration(*maxStreamTime)*time.Second, func() {
			log.Info("maxStreamTime reached, proactive VTDU reconnect")
			VTDUStream.Conn.Close()
		})
		defer t.Stop()
	}
	return LEZ.StartVTDUStream(VTDUStream, URL)
}
