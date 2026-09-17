package client

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"le-ezviz-vs/api"
	"le-ezviz-vs/logging"
	"net"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

const EZLifeURL = "ezvizlife.com"

var TerminalName = "LE-EZ"
var log = logging.Log

const (
	USE_API_URL = iota
	USE_AUTH_URL
)

var defaultHeaders = http.Header{
	"featureCode":   []string{""},
	"clientType":    []string{"9"},
	"clientVersion": []string{"2,5,1,2109068"},
	"customNo":      []string{"1000001"},
	"clientNo":      []string{"shipin7"},
	"appId":         []string{"ys7"},
	"User-Agent":    []string{""},
}

type LE_EZVIZ_Client struct {
	ClientType    int
	ClientNo      string
	FeatureCode   string
	TerminalName  string
	Headers       http.Header
	Client        *http.Client
	LoginResponse *V3_Auth_Login_Response
	APIServerInfo *ServerInfoGetResponse
	VTDUTokens    *VTDU_TokenV2
	Email         string
	Password      string
	Region        string
	API_URL       string
	AUTH_URL      string
	StreamOut     io.Writer
	streamMu      sync.Mutex
	liveConns     []net.Conn
	stopN         int32
}

func (LEZ *LE_EZVIZ_Client) TrackConn(c net.Conn) {
	LEZ.streamMu.Lock()
	LEZ.liveConns = append(LEZ.liveConns, c)
	LEZ.streamMu.Unlock()
}

func dialTCP(addr string) (net.Conn, error) {
	d := net.Dialer{Timeout: 8 * time.Second, KeepAlive: 20 * time.Second}
	sock, err := d.Dial("tcp", addr)
	if err != nil {
		return nil, err
	}
	if tc, ok := sock.(*net.TCPConn); ok {
		_ = tc.SetNoDelay(true)
	}
	return sock, nil
}

func (LEZ *LE_EZVIZ_Client) InterruptStream() {
	atomic.StoreInt32(&LEZ.stopN, 1)
	LEZ.streamMu.Lock()
	conns := append([]net.Conn(nil), LEZ.liveConns...)
	LEZ.streamMu.Unlock()
	for _, c := range conns {
		_ = c.Close()
	}
}

func (LEZ *LE_EZVIZ_Client) BeginStream() {
	atomic.StoreInt32(&LEZ.stopN, 0)
}

func (LEZ *LE_EZVIZ_Client) StreamInterrupted() bool {
	return atomic.LoadInt32(&LEZ.stopN) != 0
}

func (LEZ *LE_EZVIZ_Client) DropConns() {
	LEZ.streamMu.Lock()
	LEZ.liveConns = nil
	LEZ.streamMu.Unlock()
}

func NewLE_EZVIZ_Client(email, password, region, featurecode, terminalname, clientNo string, timeoutSeconds int) (*LE_EZVIZ_Client, error) {
	lez := &LE_EZVIZ_Client{
		Email: email, Password: GetMd5(password), Region: region,
		FeatureCode: featurecode, TerminalName: base64.StdEncoding.EncodeToString([]byte(terminalname)),
		ClientType: 9, ClientNo: clientNo,
	}
	if v, ok := Regions[region]; ok {
		if region != "Russia" {
			lez.API_URL = "https://api" + v + "." + EZLifeURL
		} else {
			lez.API_URL = "https://api.ezvizru.com"
		}
	}
	lez.Headers = defaultHeaders.Clone()
	lez.Headers["featureCode"] = []string{featurecode}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, err
	}
	lez.Client = &http.Client{
		Timeout: time.Duration(timeoutSeconds) * time.Second,
		Jar:     jar,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	return lez, nil
}

func (LEZ *LE_EZVIZ_Client) baseURL(urltype int) (string, error) {
	switch urltype {
	case USE_API_URL:
		return LEZ.API_URL, nil
	case USE_AUTH_URL:
		if LEZ.AUTH_URL == "" {
			return "", errors.New("auth URL not initialised")
		}
		return LEZ.AUTH_URL, nil
	default:
		return "", errors.New("unknown url type")
	}
}

func (LEZ *LE_EZVIZ_Client) doAPIRequest(method, endpoint string, urltype int, body io.Reader, query string) (*http.Response, error) {
	switch method {
	case "GET", "POST", "PATCH", "PUT", "DELETE", "OPTIONS":
	default:
		return nil, errors.New("unknown http method")
	}
	base, err := LEZ.baseURL(urltype)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequest(method, base+endpoint+query, body)
	if err != nil {
		return nil, err
	}
	LEZ.Headers["Content-Type"] = []string{"application/x-www-form-urlencoded"}
	req.Header = LEZ.Headers
	resp, err := LEZ.Client.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("http not ok")
	}
	return resp, nil
}

func (LEZ *LE_EZVIZ_Client) FormURLEncodedAPIRequest(method, endpoint string, urltype int, formData map[string]string) (*http.Response, error) {
	return LEZ.doAPIRequest(method, endpoint, urltype, strings.NewReader(EncodeURLForm(formData)), "")
}

func (LEZ *LE_EZVIZ_Client) QueryEncodedAPIRequest(method, endpoint string, urltype int, queryParams map[string]string) (*http.Response, error) {
	q := ""
	if len(queryParams) > 0 {
		q = EncodeQuery(queryParams)
	}
	return LEZ.doAPIRequest(method, endpoint, urltype, nil, q)
}

func (LEZ *LE_EZVIZ_Client) V3_Login() (*V3_Auth_Login_Response, error) {
	resp, err := LEZ.FormURLEncodedAPIRequest("POST", api.V3_USER_LOGIN_V5, USE_API_URL, map[string]string{
		"account": LEZ.Email, "password": LEZ.Password,
		"featureCode": LEZ.FeatureCode,
		"cuName":      base64.StdEncoding.EncodeToString([]byte(TerminalName)),
	})
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	lr := new(V3_Auth_Login_Response)
	if err := json.NewDecoder(bytes.NewBuffer(bodyBytes)).Decode(lr); err != nil {
		return nil, err
	}
	if lr.Meta.Code == nil || *lr.Meta.Code != 200 {
		code := 0
		if lr.Meta.Code != nil {
			code = *lr.Meta.Code
		}
		log.Error("Bad meta code", zap.Int("Code", code))
		return nil, errors.New("api meta code not ok")
	}
	LEZ.Headers["sessionId"] = []string{*lr.LoginSession.SessionId}
	LEZ.LoginResponse = lr
	return lr, nil
}
