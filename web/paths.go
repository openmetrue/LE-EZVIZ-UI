package main

import "path/filepath"

func workFile(name string) string { return filepath.Join(*workDir, name) }

func fifoPath() string      { return workFile("stream.ps") }
func bridgeErrPath() string { return workFile("bridge.err") }
