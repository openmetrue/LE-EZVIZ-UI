//go:build !unix

package main

import "os"

func notifyIdleStop(chan<- os.Signal) {}

func unblockFifoWriter() {}
