package main

import "path/filepath"

func workFile(name string) string { return filepath.Join(*workDir, name) }

func fifoPath() string      { return workFile("stream.ps") }
func ffmpegLogPath() string { return workFile("ffmpeg.log") }
func bridgeErrPath() string { return workFile("bridge.err") }
