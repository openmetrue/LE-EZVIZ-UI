package main

import (
	"encoding/binary"
	"io"
	"net/http"
)

func handleLive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if streamer != nil {
		streamer.Touch()
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "stream unsupported", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher.Flush()

	id, ch := live.addPeer()
	defer live.removePeer(id)

	ctx := r.Context()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			if err := writeLiveMsg(w, flusher, msg); err != nil {
				return
			}
		}
	}
}

// writeLiveMsg frames each fMP4 fragment with a 4-byte big-endian length so the
// browser can split the stream without re-parsing the container.
func writeLiveMsg(w io.Writer, flusher http.Flusher, msg []byte) error {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(msg)))
	if _, err := w.Write(hdr[:]); err != nil {
		return err
	}
	if _, err := w.Write(msg); err != nil {
		return err
	}
	flusher.Flush()
	return nil
}
