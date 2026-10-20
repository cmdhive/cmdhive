//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Cocoa
int aitActive(void);
void aitBounce(void);
*/
import "C"

// inFront reports whether Cmdhive is the app the user is looking at.
func inFront() bool { return C.aitActive() != 0 }

// dockBounce makes Cmdhive's Dock icon jump once.
func dockBounce() { C.aitBounce() }
