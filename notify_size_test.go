//go:build windows

package main

import (
	"testing"
	"unsafe"
)

// The notification struct must match NOTIFYICONDATAW exactly, or Windows
// reads the title and text from the wrong place.
func TestNotifyIconDataSize(t *testing.T) {
	if n := unsafe.Sizeof(notifyIconData{}); n != 976 {
		t.Fatalf("NOTIFYICONDATAW is %d bytes, want 976", n)
	}
}
