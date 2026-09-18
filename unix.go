//go:build unix

package main

import (
	"os"
	"os/signal"
	"syscall"
)

func notifyIdleStop(c chan<- os.Signal) {
	signal.Notify(c, syscall.SIGUSR1)
}

// unblockFifoWriter opens and closes the output FIFO so a blocked reader is
// released when the stream never started.
func unblockFifoWriter() {
	if !*idleWait || *out == "" || *out == "-" {
		return
	}
	fd, err := syscall.Open(*out, syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return
	}
	syscall.Close(fd)
}
