//go:build darwin

package main

import (
	"io"
	"strings"
	"testing"
	"time"
)

func TestPTYRunsAndEnds(t *testing.T) {
	p, err := StartPTY([]string{"/bin/sh", "-c", "echo ait-pty-$((6*7)); exit 3"}, t.TempDir(), nil, 80, 24, true)
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	done := make(chan struct{})
	go func() { io.Copy(&out, p); close(done) }()
	code, _ := p.Wait()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
	}
	if code != 3 || !strings.Contains(out.String(), "ait-pty-42") {
		t.Fatalf("exit %d, output %q", code, out.String())
	}
	p.release()
	p.Close()
}

func TestPTYCloseKillsTheTree(t *testing.T) {
	p, err := StartPTY([]string{"/bin/sh", "-c", "sleep 30 & wait"}, t.TempDir(), nil, 80, 24, true)
	if err != nil {
		t.Fatal(err)
	}
	if err := p.Resize(100, 30); err != nil {
		t.Errorf("resize: %v", err)
	}
	waited := make(chan struct{})
	go func() { p.Wait(); close(waited) }()
	time.Sleep(300 * time.Millisecond)
	p.Close()
	select {
	case <-waited:
	case <-time.After(3 * time.Second):
		t.Fatal("the process outlived Close")
	}
}
