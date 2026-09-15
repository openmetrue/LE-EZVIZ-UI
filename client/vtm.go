package client

import (
	"errors"
	"le-ezviz-vs/ezproto"
	"net"
	"strconv"

	"go.uber.org/zap"
)

type VTMStream struct {
	Conn         net.Conn
	VTMIP        string
	VTMPort      int
	VTMPublicKey string
}

// ConnectVTM dials and tracks the socket (closed by InterruptStream).
func (LEZ *LE_EZVIZ_Client) ConnectVTM(vtmIP string, vtmPort int, vtmPublicKey string) (*VTMStream, error) {
	vs, err := LEZ.dialVTM(vtmIP, vtmPort, true)
	if err != nil {
		return nil, err
	}
	vs.VTMPublicKey = vtmPublicKey
	return vs, nil
}

// DialVTM connects without tracking so a warm idle socket survives InterruptStream.
func (LEZ *LE_EZVIZ_Client) DialVTM(vtmIP string, vtmPort int) (*VTMStream, error) {
	return LEZ.dialVTM(vtmIP, vtmPort, false)
}

func (LEZ *LE_EZVIZ_Client) dialVTM(vtmIP string, vtmPort int, track bool) (*VTMStream, error) {
	sock, err := dialTCP(vtmIP + ":" + strconv.Itoa(vtmPort))
	if err != nil {
		log.Error("Error dialing VTM", zap.Error(err))
		return nil, err
	}
	vs := &VTMStream{Conn: sock, VTMIP: vtmIP, VTMPort: vtmPort}
	if track {
		LEZ.TrackConn(sock)
	}
	return vs, nil
}

func (LEZ *LE_EZVIZ_Client) StartVTMStream(vs *VTMStream, streamURL string) (*ezproto.StreamInfoRsp, error) {
	req, err := CreateStreamInfoReq(streamURL, "")
	if err != nil {
		return nil, err
	}
	if _, err = vs.Conn.Write(EncodeVTMPacket(*req, CHAN_MSG, MSG_STREAMINFO_REQ)); err != nil {
		return nil, err
	}
	pkt := &VTMPacket{Header: make([]byte, 8)}
	if _, err = vs.Conn.Read(pkt.Header); err != nil {
		return nil, err
	}
	n, _, _, msg, err := pkt.DecodeHeader()
	if err != nil {
		return nil, err
	}
	if msg != MSG_STREAMINFO_RSP {
		return nil, errors.New("unexpected message type")
	}
	pkt.Body = make([]byte, n)
	if _, err = vs.Conn.Read(pkt.Body); err != nil {
		return nil, err
	}
	rsp, err := ParseStreamInfoRsp(pkt.Body)
	if err != nil {
		_ = vs.Conn.Close()
		return nil, err
	}
	return rsp, nil
}
