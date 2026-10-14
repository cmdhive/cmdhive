//go:build darwin && cgo

package main

/*
#cgo LDFLAGS: -framework Cocoa
#import <Cocoa/Cocoa.h>

static int aitActive(void) { return [NSApp isActive] ? 1 : 0; }
static void aitBounce(void) {
	dispatch_async(dispatch_get_main_queue(), ^{ [NSApp requestUserAttention:NSInformationalRequest]; });
}
*/
import "C"

// inFront reports whether AIT is the app the user is looking at.
func inFront() bool { return C.aitActive() != 0 }

// dockBounce makes AIT's Dock icon jump once.
func dockBounce() { C.aitBounce() }
