//go:build darwin

package main

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// There is no console window to hide on a Mac. The helper gets its own
// process group instead, so killTree can end it and what it started.
func hideConsole(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killTree returns the function that ends a started process and everything
// it started since (its process group, see hideConsole).
func killTree(pid int) func() {
	return func() {
		if syscall.Kill(-pid, syscall.SIGKILL) != nil {
			syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

func fileCreated(info os.FileInfo) time.Time {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		return time.Unix(st.Birthtimespec.Unix())
	}
	return info.ModTime()
}

// A Mac has one tree; "everywhere" means everything under /.
func fixedDrives() []string { return []string{"/"} }

// The web view's drop handler gives paths on a Mac; there is nothing to add.
func clipboardFiles() []string { return nil }

func allowForeground() {}

const windowClass = ""

func openInEditor(p string)   { exec.Command("open", "-t", p).Start() }
func openFolder(p string)     { exec.Command("open", p).Start() }
func revealInFolder(p string) { exec.Command("open", "-R", p).Start() }

// A Mac app started from the Dock or Finder gets a bare PATH (/usr/bin:/bin…),
// without Homebrew, npm or the folders the agents install themselves into.
// Take the PATH of the user's own shell, plus the usual places.
func init() {
	home, _ := os.UserHomeDir()
	have := map[string]bool{}
	var dirs []string
	add := func(list string) {
		for _, d := range filepath.SplitList(list) {
			if d != "" && !have[d] {
				have[d] = true
				dirs = append(dirs, d)
			}
		}
	}
	add(os.Getenv("PATH"))
	add(shellPath())
	add(strings.Join([]string{
		"/opt/homebrew/bin", "/opt/homebrew/sbin", "/usr/local/bin",
		filepath.Join(home, ".local", "bin"), filepath.Join(home, ".claude", "local"),
		filepath.Join(home, ".npm-global", "bin"), filepath.Join(home, ".bun", "bin"), filepath.Join(ownNodeDir(), "bin"),
	}, ":"))
	os.Setenv("PATH", strings.Join(dirs, ":"))
}

// shellPath asks the user's login shell for its PATH (nvm and friends only
// set it up there). Gives up after two seconds.
func shellPath() string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, sh, "-ilc", `printf '\n__AIT_PATH__%s__AIT_PATH__\n' "$PATH"`)
	cmd.Stdin = nil
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	_, rest, ok := bytes.Cut(out, []byte("__AIT_PATH__"))
	if !ok {
		return ""
	}
	path, _, _ := bytes.Cut(rest, []byte("__AIT_PATH__"))
	return string(path)
}

const defaultFont = "Menlo"

// shellProfiles: the user's own shell, as one "Terminal" entry.
func shellProfiles() []Profile {
	return []Profile{{ID: "cmd", Name: "Terminal", Installed: true}}
}

func shellArgv(kind string) []string {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/zsh"
	}
	return []string{sh, "-l"}
}
