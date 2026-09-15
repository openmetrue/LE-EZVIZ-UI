package client

import (
	"errors"
	"io"
	"net"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type VTDUStream struct {
	Conn         net.Conn
	VTDUIP       string
	VTDUPort     int
	VTMPublicKey string
	VTMStreamKey string
}

func (LEZ *LE_EZVIZ_Client) ConnectVTDU(vtduIP string, vtduPort int, vtmStreamKey, vtmPublicKey string) (*VTDUStream, error) {
	sock, err := dialTCP(vtduIP + ":" + strconv.Itoa(vtduPort))
	if err != nil {
		log.Error("Error dialing VTDU", zap.Error(err))
		return nil, err
	}
	vs := &VTDUStream{Conn: sock, VTDUIP: vtduIP, VTDUPort: vtduPort, VTMStreamKey: vtmStreamKey, VTMPublicKey: vtmPublicKey}
	LEZ.TrackConn(sock)
	return vs, nil
}

func (LEZ *LE_EZVIZ_Client) StartVTDUStream(vs *VTDUStream, streamURL string) error {
	req, err := CreateStreamInfoReq(streamURL, vs.VTMStreamKey)
	if err != nil {
		return err
	}
	if _, err = vs.Conn.Write(EncodeVTMPacket(*req, CHAN_MSG, MSG_STREAMINFO_REQ)); err != nil {
		return err
	}
	pkt := &VTMPacket{Header: make([]byte, 8)}
	if _, err = vs.Conn.Read(pkt.Header); err != nil {
		return err
	}
	n, _, _, msg, err := pkt.DecodeHeader()
	if err != nil {
		return err
	}
	if msg != MSG_STREAMINFO_RSP {
		return errors.New("unexpected message type")
	}
	pkt.Body = make([]byte, n)
	if _, err = vs.Conn.Read(pkt.Body); err != nil {
		return err
	}
	rsp, err := ParseStreamInfoRsp(pkt.Body)
	if err != nil {
		return err
	}
	if rsp.Result == nil {
		return errors.New("result is nil")
	}
	if *rsp.Result != 0 {
		return LEZ.CheckRetcode(*rsp.Result)
	}
	if rsp.Streamssn == nil {
		return errors.New("streamssn nil")
	}
	if err = SendKeepAlive(vs.Conn, *rsp.Streamssn); err != nil {
		return err
	}
	out := LEZ.StreamOut
	if out == nil {
		return errors.New("no stream output configured")
	}
	go CreateKeepAliveTicker(vs.Conn, *rsp.Streamssn)

	gotMedia := false
	for {
		pkt := &VTMPacket{Header: make([]byte, 8)}
		_ = vs.Conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		if _, err = io.ReadFull(vs.Conn, pkt.Header); err != nil {
			return err
		}
		n, ch, seq, msg, err := pkt.DecodeHeader()
		if err != nil {
			log.Error("Header decode failed, reconnecting", zap.Error(err))
			return err
		}
		pkt.Body = make([]byte, n)
		if _, err = io.ReadFull(vs.Conn, pkt.Body); err != nil {
			return err
		}
		if msg == MSG_KEEPALIVE_RSP {
			continue
		}
		if ch == CHAN_STREAM {
			gotMedia = true
			if _, err := out.Write(pkt.Body); err != nil {
				return err
			}
			continue
		}
		if ch == CHAN_MSG && seq == 0 && gotMedia {
			log.Info("Stream ended by server")
			return nil
		}
	}
}

func SendKeepAlive(sock net.Conn, streamSSN string) error {
	b, err := CreateStreamKeepaliveReq(streamSSN)
	if err != nil {
		return err
	}
	_, err = sock.Write(EncodeVTMPacket(*b, CHAN_MSG, MSG_KEEPALIVE_REQ))
	return err
}

func CreateKeepAliveTicker(sock net.Conn, streamSSN string) {
	_ = SendKeepAlive(sock, streamSSN)
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for range t.C {
		if err := SendKeepAlive(sock, streamSSN); err != nil {
			log.Error("Error sending keep alive", zap.Error(err))
			return
		}
	}
}
