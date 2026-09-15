package main

import "path/filepath"

func workFile(name string) string { return filepath.Join(*workDir, name) }

func hlsDir() string        { return workFile("hls") }
func fifoPath() string      { return workFile("stream.ps") }
func recDir() string        { return workFile("recordings") }
func ffmpegLogPath() string { return workFile("ffmpeg.log") }
func bridgeErrPath() string { return workFile("bridge.err") }
func playlistPath() string  { return filepath.Join(hlsDir(), playlistName) }

func hlsFile(name string) string {
	return filepath.Join(hlsDir(), filepath.Base(name))
}
