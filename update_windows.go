//go:build windows

package main

import "os/exec"

// Cmdhive was called AIT, and copies from then look for AIT-setup.exe: every
// release publishes the same installer under both names (scripts/release.sh).
const (
	installerAsset = "Cmdhive-setup.exe"
	checksumAsset  = "Cmdhive-setup.exe.sha256"
)

func updateFile(version string) string { return "Cmdhive-setup-" + version + ".exe" }

// runUpdate runs a verified installer: silently (the user already said yes),
// and with relaunch, starting Cmdhive again when done.
func runUpdate(path string, relaunch bool) error {
	if relaunch {
		return exec.Command(path, "/S", "/RELAUNCH").Start()
	}
	return exec.Command(path, "/S").Start()
}
