package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"le-ezviz-vs/api"
	"strconv"

	"go.uber.org/zap"
)

type ServerInfoGetResponse struct {
	ServerResp *struct {
		AuthAddr *string `json:"authAddr"`
	} `json:"serverResp"`
}

func (LEZ *LE_EZVIZ_Client) GetServerInfo() (*ServerInfoGetResponse, error) {
	resp, err := LEZ.FormURLEncodedAPIRequest("POST", api.API_SERVER_INFO_GET, USE_API_URL, map[string]string{
		"sessionId":  *LEZ.LoginResponse.LoginSession.SessionId,
		"clientType": strconv.Itoa(LEZ.ClientType),
	})
	if err != nil {
		log.Error("GetServerInfo Request error", zap.Error(err))
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Error("Error reading response body", zap.Error(err))
		return nil, err
	}
	info := new(ServerInfoGetResponse)
	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(info); err != nil {
		log.Error("Error decoding JSON", zap.Error(err))
		return nil, err
	}
	if info.ServerResp == nil || info.ServerResp.AuthAddr == nil || *info.ServerResp.AuthAddr == "" {
		return nil, errors.New("authAddr missing in server info")
	}
	LEZ.APIServerInfo = info
	LEZ.AUTH_URL = *info.ServerResp.AuthAddr
	return info, nil
}
