//go:build linux

package bao

import (
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestTLSFileRejectsFIFOWithoutBlocking(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ca-input")
	if err := syscall.Mkfifo(path, 0600); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := readTLSFile(path); done <- e }()
	select {
	case e := <-done:
		if e == nil {
			t.Fatal("FIFO accepted")
		}
	case <-time.After(100 * time.Millisecond):
		// Unblock the buggy implementation before failing; no leaked goroutine.
		fd, e := syscall.Open(path, syscall.O_RDWR|syscall.O_NONBLOCK, 0)
		if e != nil {
			t.Fatal(e)
		}
		defer syscall.Close(fd)
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("FIFO reader remained stuck")
		}
		t.Fatal("non-regular TLS file blocked before validation")
	}
}
