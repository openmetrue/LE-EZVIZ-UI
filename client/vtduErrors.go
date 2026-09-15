package client

import (
	"errors"
	"fmt"
)

var vtduRetcodes = map[int32]string{
	5000: "5000: server exception",
	5400: "5400: wrong client parameters",
	5402: "5402: the recording file cannot be found on the device",
	5403: "5403: opcode signaling key does not match device",
	5404: "5404: device offline",
	5405: "5405: signaling timeout/CAS response timeout after 10s",
	5406: "5406: token invalid",
	5407: "5407: url is malformed",
	5409: "5409: device has privacy protection on",
	5411: "5411: token/user does not permission",
	5412: "5412: session does not exist",
	5413: "5413: token validation failed",
	5415: "5415: device wrong channel",
	5416: "5416: resources limited",
	5451: "5451: stream unsupported",
	5452: "5452: device link to stream server failed",
	5454: "5454: no session about device streaming",
	5455: "5455: device channel is not associated",
	5456: "5456: device channel associated device is offline",
	5457: "5457: client doesn't support E2EE",
	5458: "5458: device does not support concurrent ECDH password",
	5459: "5459: VTDU failed to process ECDH encryption",
	5491: "5491: The same request is being processed and will now be rejected",
	5492: "5492: commands not support by device",
	5500: "5500: server processing failed",
	5503: "5503: VTM failed to allocate VTDU",
	5504: "5504: streaming VTDUs limit reached for user",
	5544: "5544: device returns no video source",
	5545: "5545: video sharing time has ended",
	5546: "5546: VTDU concurrent channels limit reached",
	6518: "6518: device packet too large",
	6519: "6519: device network link is unstable",
	6520: "6520: unstable network",
	7005: "7005: VTDU disconnected",
}

func (LEZ *LE_EZVIZ_Client) CheckRetcode(ret int32) error {
	if msg, ok := vtduRetcodes[ret]; ok {
		return errors.New(msg)
	}
	return fmt.Errorf("Response result not 0 instead got: %d", ret)
}
