//go:build darwin

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	installerAsset = "AIT-mac.zip"
	checksumAsset  = "AIT-mac.zip.sha256"
)

func updateFile(version string) string { return "AIT-mac-" + version + ".zip" }

// runUpdate replaces the AIT.app that is running with the one in a verified
// zip. A helper waits for this AIT to quit, swaps the bundle, and (with
// relaunch) opens the new one.
func runUpdate(zip string, relaunch bool) error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	app, _, ok := strings.Cut(exe, ".app/Contents/")
	if !ok {
		return errors.New("AIT is not running from an app bundle, so it cannot update itself")
	}
	app += ".app"
	if f, err := os.CreateTemp(filepath.Dir(app), ".ait-write-test"); err != nil {
		return fmt.Errorf("AIT cannot write to %s: %w", filepath.Dir(app), err)
	} else {
		f.Close()
		os.Remove(f.Name())
	}
	stage, err := os.MkdirTemp("", "ait-update-")
	if err != nil {
		return err
	}
	if out, err := exec.Command("ditto", "-x", "-k", zip, stage).CombinedOutput(); err != nil {
		return fmt.Errorf("could not unpack the update: %s", lastLines(strings.TrimSpace(string(out)), 2))
	}
	next := filepath.Join(stage, "AIT.app")
	if !fileExists(filepath.Join(next, "Contents", "MacOS")) {
		return errors.New("the update does not contain AIT.app")
	}
	script := `while kill -0 "$1" 2>/dev/null; do sleep 0.3; done
rm -rf "$2" && ditto "$3" "$2" && xattr -cr "$2"
rm -rf "$4"
[ "$5" = 1 ] && open "$2"`
	flag := "0"
	if relaunch {
		flag = "1"
	}
	cmd := exec.Command("/bin/sh", "-c", script, "sh", fmt.Sprint(os.Getpid()), app, next, stage, flag)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	return cmd.Start()
}
