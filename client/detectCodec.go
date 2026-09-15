package client

const (
	TRANS_UNKNOWN = iota
	TRANS_MPEG4
	TRANS_MPEG_TS
	TRANS_MPEG_PS
	TRANS_RTP
)

func DetectTransport(buf []byte) int {
	if len(buf) < 4 {
		return TRANS_UNKNOWN
	}
	if buf[0] == 0 && buf[1] == 0 && buf[2] == 1 && buf[3] == 0xba {
		return TRANS_MPEG_PS
	}
	if buf[0] == 0x47 {
		return TRANS_MPEG_TS
	}
	if buf[0]>>6 == 2 {
		return TRANS_RTP
	}
	return TRANS_UNKNOWN
}
