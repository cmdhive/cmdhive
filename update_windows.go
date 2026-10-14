//go:build windows

package main

import "os/exec"

const (
	installerAsset = "AIT-setup.exe"
	checksumAsset  = "AIT-setup.exe.sha256"
)

func updateFile(version string) string { return "AIT-setup-" + version + ".exe" }

// runUpdate runs a verified installer: silently (the user already said yes),
// and with relaunch, starting AIT again when done.
func runUpdate(path string, relaunch bool) error {
	if relaunch {
		return exec.Command(path, "/S", "/RELAUNCH").Start()
	}
	return exec.Command(path, "/S").Start()
}
