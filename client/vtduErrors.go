package client

import (
	"errors"
	"fmt"
)

// Common VTDU errors; unknown codes still return the numeric result.
var vtduRetcodes = map[int32]string{
	5404: "device offline",
	5405: "signaling timeout",
	5406: "token invalid",
	5411: "token unauthorised",
	5416: "resources limited",
	5452: "device link to stream server failed",
	5503: "VTM failed to allocate VTDU",
	5504: "streaming VTDU limit reached",
	5544: "device returns no video source",
	7005: "VTDU disconnected",
}

func (LEZ *LE_EZVIZ_Client) CheckRetcode(ret int32) error {
	if msg, ok := vtduRetcodes[ret]; ok {
		return errors.New(msg)
	}
	return fmt.Errorf("vtdu result %d", ret)
}
