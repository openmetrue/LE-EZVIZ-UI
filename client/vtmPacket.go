package client

import (
	"encoding/binary"
	"errors"
)

type VTMPacket struct {
	Header []byte
	Body   []byte
}

func EncodeVTMPacket(data []byte, chanID byte, messageCode int) []byte {
	header := make([]byte, 8)
	header[0] = 0x24
	header[1] = chanID
	binary.BigEndian.PutUint16(header[2:4], uint16(len(data)))
	binary.BigEndian.PutUint16(header[6:8], uint16(messageCode))
	return append(header, data...)
}

func (p *VTMPacket) DecodeHeader() (length uint16, channel byte, sequence uint16, message uint16, err error) {
	if p.Header[0] != 0x24 {
		return 0, 0, 0, 0, errors.New("magic not found")
	}
	switch p.Header[1] {
	case CHAN_MSG, CHAN_STREAM:
	case 0x0a, 0x0b:
		return 0, 0, 0, 0, errors.New("encrypted channel currently unsupported")
	default:
		return 0, 0, 0, 0, errors.New("unknown channel")
	}
	length = binary.BigEndian.Uint16(p.Header[2:4])
	sequence = binary.BigEndian.Uint16(p.Header[4:6])
	message = binary.BigEndian.Uint16(p.Header[6:8])
	return length, p.Header[1], sequence, message, nil
}
