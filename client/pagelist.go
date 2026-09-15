package client

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"le-ezviz-vs/api"
	"net/http"
	"strconv"

	"go.uber.org/zap"
)

type PageListResponse struct {
	ResourceInfos *[]Resource            `json:"resourceInfos"`
	VTM           map[string]VTMResource `json:"VTM"`
	DeviceInfos   *[]DeviceInfos         `json:"deviceInfos"`
}

type VTMResource struct {
	ExternalIP string `json:"externalIp"`
	Port       int    `json:"port"`
	PublicKey  struct {
		Key string `json:"key"`
	} `json:"publicKey"`
}

type Resource struct {
	ResourceID   string `json:"resourceId"`
	DeviceSerial string `json:"deviceSerial"`
	StreamBizUrl string `json:"streamBizUrl"`
}

type DeviceInfos struct {
	Name          string `json:"name"`
	DeviceSerial  string `json:"deviceSerial"`
	ChannelNumber int    `json:"channelNumber"`
}

// DeviceStatus is compact STATUS/WIFI from the cloud pagelist (does not wake the camera).
type DeviceStatus struct {
	Battery          string `json:"battery"`
	Online           bool   `json:"online"`
	WifiSignal       int    `json:"wifi_signal"`
	WifiSSID         string `json:"wifi_ssid"`
	PirStatus        int    `json:"pir"`
	UpgradeAvailable int    `json:"upgrade_available"`
	KeepAliveSec     int    `json:"keep_alive_sec"`
}

func (LEZ *LE_EZVIZ_Client) GetDeviceStatus(deviceSerial string) (*DeviceStatus, error) {
	raw, err := LEZ.statusRawJSON()
	if err != nil {
		return nil, err
	}
	var parsed struct {
		STATUS map[string]struct {
			PirStatus        int `json:"pirStatus"`
			UpgradeAvailable int `json:"upgradeAvailable"`
			Optionals        struct {
				PowerRemaining    string `json:"powerRemaining"`
				OnlineStatus      string `json:"OnlineStatus"`
				BatteryWorkStatus string `json:"Battery_WorkStatus"`
			} `json:"optionals"`
		} `json:"STATUS"`
		WIFI map[string]struct {
			Signal int    `json:"signal"`
			SSID   string `json:"ssid"`
		} `json:"WIFI"`
	}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil, err
	}
	st, ok := parsed.STATUS[deviceSerial]
	if !ok {
		return nil, fmt.Errorf("device %s not found in STATUS", deviceSerial)
	}
	ds := &DeviceStatus{
		Battery:          st.Optionals.PowerRemaining,
		Online:           st.Optionals.OnlineStatus == "1",
		PirStatus:        st.PirStatus,
		UpgradeAvailable: st.UpgradeAvailable,
	}
	var bws struct {
		KeepAlive int `json:"KeepAlive"`
	}
	if err := json.Unmarshal([]byte(st.Optionals.BatteryWorkStatus), &bws); err == nil {
		ds.KeepAliveSec = bws.KeepAlive
	}
	if w, ok := parsed.WIFI[deviceSerial]; ok {
		ds.WifiSignal = w.Signal
		ds.WifiSSID = w.SSID
	}
	return ds, nil
}

func (LEZ *LE_EZVIZ_Client) statusRawJSON() (string, error) {
	resp, err := LEZ.pagelist("STATUS,WIFI")
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func (LEZ *LE_EZVIZ_Client) GetPageList() (*PageListResponse, error) {
	resp, err := LEZ.pagelist("VTM")
	if err != nil {
		log.Error("GetPageList Request error", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response body", zap.Error(err))
		return nil, err
	}
	PageList := new(PageListResponse)
	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(PageList); err != nil {
		log.Error("Error decoding JSON", zap.Error(err))
		return nil, err
	}
	return PageList, nil
}

func (LEZ *LE_EZVIZ_Client) pagelist(filter string) (*http.Response, error) {
	return LEZ.QueryEncodedAPIRequest("GET", api.V3_USERDEVICES_V1_RESOURCES_PAGELIST, USE_API_URL, map[string]string{
		"sessionId":     *LEZ.LoginResponse.LoginSession.SessionId,
		"clientType":    strconv.Itoa(LEZ.ClientType),
		"clientNo":      LEZ.ClientNo,
		"clientVersion": "2,5,1,2109068",
		"groupId":       "-1",
		"limit":         "50",
		"offset":        "0",
		"filter":        filter,
	})
}
