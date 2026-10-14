package main

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
)

// Installing the AI tools from the setup wizard. Claude Code has its own
// installer; ChatGPT (Codex) and Gemini come from npm, which needs Node.js,
// installed first from nodejs.org when it is missing.

var npmPackages = map[string]string{
	"codex":  "@openai/codex",
	"gemini": "@google/gemini-cli",
}

// InstallAgents installs the given tools one after another, reporting each
// as "install:progress" (id, state: installing | done | failed, message).
func (a *App) InstallAgents(ids []string) {
	for _, id := range ids {
		p := registry[id]
		if p == nil || p.Command() != nil {
			continue
		}
		a.emit("install:progress", id, "installing", "")
		err := installAgent(id)
		if err == nil && p.Command() == nil {
			err = errors.New("installed, but AIT can't find it yet; restart AIT")
		}
		if err != nil {
			a.emit("install:progress", id, "failed", err.Error())
			continue
		}
		a.emit("install:progress", id, "done", "")
	}
}

func installAgent(id string) error {
	if id == "claude" {
		if runtime.GOOS != "windows" {
			return run("/bin/sh", "-c", "curl -fsSL https://claude.ai/install.sh | bash")
		}
		return run("powershell.exe", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", "irm https://claude.ai/install.ps1 | iex")
	}
	pkg := npmPackages[id]
	if pkg == "" {
		return fmt.Errorf("AIT can't install %s", id)
	}
	npm, err := ensureNode()
	if err != nil {
		return err
	}
	return run(npm, "install", "-g", pkg)
}

// run executes a hidden console command, returning its last output lines on
// failure.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	hideConsole(cmd)
	out, err := cmd.CombinedOutput()
	if err != nil {
		if msg := lastLines(strings.TrimSpace(string(out)), 3); msg != "" {
			return errors.New(msg)
		}
		return err
	}
	return nil
}

// minNode is the oldest Node.js the tools run on (Gemini CLI needs 20).
const minNode = 20

// ensureNode returns npm, installing Node.js LTS when it is missing or older
// than minNode.
func ensureNode() (string, error) {
	if runtime.GOOS != "windows" {
		return ensureNodeMac()
	}
	dir := filepath.Join(os.Getenv("ProgramFiles"), "nodejs")
	npm := npmBinary()
	if npm != "" && nodeMajor(lookBinary("node", filepath.Join(dir, "node.exe"))) >= minNode {
		return npm, nil
	}
	idx, err := fetch("https://nodejs.org/dist/index.json", 4<<20)
	if err != nil {
		return "", fmt.Errorf("could not reach nodejs.org: %w", err)
	}
	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"`
	}
	json.Unmarshal(idx, &releases)
	ver := ""
	for _, r := range releases {
		if s, ok := r.LTS.(string); ok && s != "" {
			ver = r.Version
			break
		}
	}
	if ver == "" {
		return "", errors.New("could not find the current Node.js release")
	}
	file := "node-" + ver + "-x64.msi"
	base := "https://nodejs.org/dist/" + ver + "/"
	sums, err := fetch(base+"SHASUMS256.txt", 1<<20)
	if err != nil {
		return "", fmt.Errorf("could not get Node.js checksums: %w", err)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == file {
			want = f[0]
		}
	}
	data, err := fetch(base+file, 200<<20)
	if err != nil {
		return "", fmt.Errorf("Node.js download failed: %w", err)
	}
	if got := sha256.Sum256(data); want == "" || hex.EncodeToString(got[:]) != want {
		return "", errors.New("the Node.js download does not match its checksum; not installing it")
	}
	msi := filepath.Join(os.TempDir(), file)
	if err := os.WriteFile(msi, data, 0o644); err != nil {
		return "", err
	}
	defer os.Remove(msi)
	// Node installs for the whole PC, so Windows asks for permission.
	if err := exec.Command("msiexec.exe", "/i", msi, "/passive", "/norestart").Run(); err != nil {
		return "", fmt.Errorf("Node.js was not installed: %w", err)
	}
	// This process started without Node on its PATH; the agents need it.
	os.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	npm = filepath.Join(dir, "npm.cmd")
	if !fileExists(npm) {
		return "", errors.New("Node.js installed, but npm was not found")
	}
	return npm, nil
}

// nodeMajor is node's major version ("v18.20.1" -> 18), 0 when unknown.
func nodeMajor(node string) int {
	if node == "" {
		return 0
	}
	cmd := exec.Command(node, "--version")
	hideConsole(cmd)
	out, _ := cmd.Output()
	v := strings.TrimPrefix(strings.TrimSpace(string(out)), "v")
	n, _ := strconv.Atoi(strings.Split(v, ".")[0])
	return n
}

// npmBinary finds npm (npm.cmd on Windows).
func npmBinary() string {
	if runtime.GOOS != "windows" {
		return lookBinary("npm")
	}
	return lookBinary("npm.cmd", filepath.Join(os.Getenv("ProgramFiles"), "nodejs", "npm.cmd"))
}

// ownNodeDir is where AIT keeps a Node.js of its own on a Mac, for a Mac that
// has none: it needs no password and no Homebrew.
func ownNodeDir() string {
	d, _ := os.UserConfigDir()
	return filepath.Join(d, "AIT", "node")
}

// ensureNodeMac returns npm, unpacking the official Node.js LTS into AIT's own
// folder when the Mac has no Node.js (or one older than minNode).
func ensureNodeMac() (string, error) {
	npm := npmBinary()
	if npm != "" && nodeMajor(lookBinary("node")) >= minNode {
		return npm, nil
	}
	idx, err := fetch("https://nodejs.org/dist/index.json", 4<<20)
	if err != nil {
		return "", fmt.Errorf("could not reach nodejs.org: %w", err)
	}
	var releases []struct {
		Version string `json:"version"`
		LTS     any    `json:"lts"`
	}
	json.Unmarshal(idx, &releases)
	ver := ""
	for _, r := range releases {
		if s, ok := r.LTS.(string); ok && s != "" {
			ver = r.Version
			break
		}
	}
	if ver == "" {
		return "", errors.New("could not find the current Node.js release")
	}
	arch := map[string]string{"arm64": "arm64", "amd64": "x64"}[runtime.GOARCH]
	file := "node-" + ver + "-darwin-" + arch + ".tar.gz"
	base := "https://nodejs.org/dist/" + ver + "/"
	sums, err := fetch(base+"SHASUMS256.txt", 1<<20)
	if err != nil {
		return "", fmt.Errorf("could not get Node.js checksums: %w", err)
	}
	want := ""
	for _, line := range strings.Split(string(sums), "\n") {
		if f := strings.Fields(line); len(f) == 2 && f[1] == file {
			want = f[0]
		}
	}
	data, err := fetch(base+file, 200<<20)
	if err != nil {
		return "", fmt.Errorf("Node.js download failed: %w", err)
	}
	if got := sha256.Sum256(data); want == "" || hex.EncodeToString(got[:]) != want {
		return "", errors.New("the Node.js download does not match its checksum; not installing it")
	}
	dir := ownNodeDir()
	os.RemoveAll(dir)
	if err := untarStripped(data, dir); err != nil {
		return "", fmt.Errorf("could not unpack Node.js: %w", err)
	}
	os.Setenv("PATH", filepath.Join(dir, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
	npm = filepath.Join(dir, "bin", "npm")
	if !fileExists(npm) {
		return "", errors.New("Node.js unpacked, but npm was not found")
	}
	return npm, nil
}

// untarStripped unpacks a .tar.gz into dir without its top folder.
func untarStripped(data []byte, dir string) error {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return err
	}
	tr := tar.NewReader(zr)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		_, rel, ok := strings.Cut(h.Name, "/")
		if !ok || rel == "" {
			continue
		}
		dst := filepath.Join(dir, filepath.FromSlash(rel))
		if !strings.HasPrefix(dst, dir+string(os.PathSeparator)) {
			return errors.New("unsafe path in the archive")
		}
		switch h.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(dst, 0o755)
		case tar.TypeSymlink:
			os.MkdirAll(filepath.Dir(dst), 0o755)
			if err := os.Symlink(h.Linkname, dst); err != nil {
				return err
			}
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(dst), 0o755)
			f, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(h.Mode)&0o777)
			if err != nil {
				return err
			}
			_, err = io.Copy(f, tr)
			f.Close()
			if err != nil {
				return err
			}
		}
	}
}
