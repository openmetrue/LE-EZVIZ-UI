package client

import (
	"errors"
	"io"
	"net"
	"os"
	"strconv"
	"time"

	"go.uber.org/zap"
)

type VTDUStream struct {
	Conn          net.Conn
	VTDUIP        string
	VTDUPort      int
	VTDUStreamSSN string
	VTMPublicKey  string
	VTMStreamKey  string
	SessionKey    string
	MasterKey     string
	Transport     int
}

func (LEZ *LE_EZVIZ_Client) ConnectVTDU(vtduIP string, vtduPort int, vtmStreamKey, vtmPublicKey string) (*VTDUStream, error) {
	sock, err := dialTCP(vtduIP + ":" + strconv.Itoa(vtduPort))
	if err != nil {
		log.Error("Error dialing VTDU", zap.Error(err))
		return nil, err
	}
	VS := &VTDUStream{Conn: sock, VTDUIP: vtduIP, VTDUPort: vtduPort, VTMStreamKey: vtmStreamKey, VTMPublicKey: vtmPublicKey}
	LEZ.TrackConn(sock)
	return VS, nil
}

func (LEZ *LE_EZVIZ_Client) streamWriter() (io.Writer, func(), error) {
	if LEZ.PipeMode {
		return LEZ.StreamOut, func() {}, nil
	}
	path := LEZ.StreamFile
	if path == "" {
		path = "stream"
	}
	f, err := os.Create(path)
	if err != nil {
		return nil, nil, err
	}
	return f, func() { _ = f.Close() }, nil
}

func (LEZ *LE_EZVIZ_Client) StartVTDUStream(VTDUstream *VTDUStream, StreamURL string) error {
	StreamReq, err := CreateStreamInfoReq(StreamURL, VTDUstream.VTMStreamKey)
	if err != nil {
		log.Error("Error creating StreamInfoReq", zap.Error(err))
		return err
	}
	EncodedPacket := EncodeVTMPacket(*StreamReq, CHAN_MSG, MSG_STREAMINFO_REQ)
	log.Sugar().Debugf("bytes: %x", EncodedPacket)
	if _, err = VTDUstream.Conn.Write(EncodedPacket); err != nil {
		log.Error("Error writing to TCP sock", zap.Error(err))
		return err
	}
	Packet := new(VTMPacket)
	Packet.Header = make([]byte, 8)
	if _, err = VTDUstream.Conn.Read(Packet.Header); err != nil {
		log.Error("Error reading from TCP sock", zap.Error(err))
		return err
	}
	Len, _, _, Msg, err := Packet.DecodeHeader()
	if err != nil {
		log.Error("Error decoding packet header", zap.Error(err))
		return err
	}
	if Msg != MSG_STREAMINFO_RSP {
		return errors.New("Unexpected message type in buffer area")
	}
	Packet.Body = make([]byte, Len)
	if _, err = VTDUstream.Conn.Read(Packet.Body); err != nil {
		log.Error("Error reading from TCP sock", zap.Error(err))
		return err
	}
	log.Debug("0x13c received StreamInfoRsp")
	log.Sugar().Debugf("RspBytes:%x", Packet.Body)
	Rsp, err := ParseStreamInfoRsp(Packet.Body)
	if err != nil {
		log.Error("Closing VTDU Stream, error on StreamInfoRsp", zap.Error(err))
		return err
	}
	if Rsp.Result == nil {
		return errors.New("Result is nil")
	}
	if *Rsp.Result != 0 {
		return LEZ.CheckRetcode(*Rsp.Result)
	}
	if Rsp.Streamssn == nil {
		return errors.New("Streamssn nil")
	}
	if err = SendKeepAlive(VTDUstream.Conn, *Rsp.Streamssn); err != nil {
		log.Error("Error sending keepalive to VTDU", zap.Error(err))
		return err
	}

	streamFile, closeOut, err := LEZ.streamWriter()
	if err != nil {
		return err
	}
	defer closeOut()

	go CreateKeepAliveTicker(VTDUstream.Conn, *Rsp.Streamssn)
	gotMedia := false
	for {
		Packet := new(VTMPacket)
		Packet.Header = make([]byte, 8)
		_ = VTDUstream.Conn.SetReadDeadline(time.Now().Add(15 * time.Second))
		if _, err = io.ReadFull(VTDUstream.Conn, Packet.Header); err != nil {
			log.Error("Error reading from TCP sock", zap.Error(err))
			return err
		}
		Len, Chan, Seq, Msg, err := Packet.DecodeHeader()
		if err != nil {
			log.Error("Header decode failed, reconnecting", zap.Error(err))
			return err
		}
		log.Debug("Sequence", zap.Uint16("Seq", Seq))
		Packet.Body = make([]byte, Len)
		if _, err = io.ReadFull(VTDUstream.Conn, Packet.Body); err != nil {
			log.Error("Error reading body from TCP sock", zap.Error(err))
			return err
		}
		if Msg == MSG_KEEPALIVE_RSP {
			log.Debug("Keepalive responded")
			continue
		}
		if Chan == CHAN_STREAM {
			if VTDUstream.Transport == TRANS_UNKNOWN {
				VTDUstream.Transport = DetectTransport(Packet.Body)
			}
			switch VTDUstream.Transport {
			case TRANS_MPEG_PS:
				gotMedia = true
				if _, err := streamFile.Write(Packet.Body); err != nil {
					return err
				}
			case TRANS_RTP:
				gotMedia = true
				payload, err := LEZ.DecodeRTP(Packet.Body)
				if err != nil {
					log.Error("Error decoding RTP", zap.Error(err))
					continue
				}
				if len(payload) == 0 {
					continue
				}
				if _, err = streamFile.Write(payload); err != nil {
					return err
				}
			}
			continue
		}
		// Control channel after media: server ended the stream.
		if Chan == CHAN_MSG && Seq == 0 && gotMedia {
			log.Info("Stream ended by server")
			return nil
		}
	}
}

func SendKeepAlive(Sock net.Conn, StreamSSN string) error {
	bytes, err := CreateStreamKeepaliveReq(StreamSSN)
	if err != nil {
		return err
	}
	_, err = Sock.Write(EncodeVTMPacket(*bytes, CHAN_MSG, MSG_KEEPALIVE_REQ))
	return err
}

func CreateKeepAliveTicker(Sock net.Conn, StreamSSN string) {
	_ = SendKeepAlive(Sock, StreamSSN)
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()
	for range ticker.C {
		if err := SendKeepAlive(Sock, StreamSSN); err != nil {
			log.Error("Error sending keep alive", zap.Error(err))
			return
		}
	}
}
