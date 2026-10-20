//go:build darwin && !cgo

package main

// Without cgo (a type-check build) there is no way to ask; Cmdhive is never "in front".
func inFront() bool { return false }

func dockBounce() {}
