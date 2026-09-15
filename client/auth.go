package client

type V3_Auth_Login_Response struct {
	LoginSession struct {
		SessionId *string `json:"sessionId"`
	} `json:"loginSession"`
	Meta struct {
		Code *int `json:"code"`
	} `json:"meta"`
}
