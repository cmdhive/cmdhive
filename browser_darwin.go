//go:build darwin

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

// On a Mac the AIs' browser is the Mac's own Chrome (or Edge, Brave, Chromium),
// started with a private profile and a debugging port that Playwright drives.
// It is a separate app, not Cmdhive's live-stream window with the steps column:
// the steps and the questions for the user appear in the chat instead.

type browserView struct {
	key  string
	port int
	dir  string // the browser's user data folder
	temp bool   // removed when the view closes
	cmd  *exec.Cmd

	mu    sync.Mutex
	gone  bool
	url   string
	title string
}

func (v *browserView) isGone() bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.gone
}

// chromeApp finds a Chromium-based browser installed on this Mac.
func chromeApp() string {
	home, _ := os.UserHomeDir()
	for _, app := range []string{"Google Chrome", "Microsoft Edge", "Brave Browser", "Chromium", "Google Chrome Canary"} {
		for _, root := range []string{"/Applications", filepath.Join(home, "Applications")} {
			if p := filepath.Join(root, app+".app", "Contents", "MacOS", app); fileExists(p) {
				return p
			}
		}
	}
	return ""
}

// browserEnsureView starts the chat's browser on first use, and again when
// the user closed it.
func (a *App) browserEnsureView(l *browserLog) error {
	l.viewMu.Lock()
	defer l.viewMu.Unlock()
	if l.view != nil && !l.view.isGone() {
		return nil
	}
	l.view = nil
	exe := chromeApp()
	if exe == "" {
		return errors.New("Cmdhive's browser needs Google Chrome (or Edge, Brave, Chromium) on this Mac")
	}
	// The user chose this when the browser was first opened (browserChooseMode).
	// One signed-in profile can serve one window at a time, so a second chat
	// that wants it gets a private one instead.
	dir, temp := a.store.browserProfileDir(), false
	if a.browserModeOf(l) != browserKeep || a.profileBusy() {
		var err error
		if dir, err = os.MkdirTemp(a.browserTmp(), "view-"); err != nil {
			return err
		}
		temp = true
	}
	os.MkdirAll(dir, 0o755)
	cmd := exec.Command(exe,
		fmt.Sprintf("--remote-debugging-port=%d", l.port), "--user-data-dir="+dir,
		"--no-first-run", "--no-default-browser-check", "--window-size=1280,860", "about:blank")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := cmd.Start(); err != nil {
		return err
	}
	v := &browserView{key: l.key, port: l.port, dir: dir, temp: temp, cmd: cmd}
	go func() {
		cmd.Wait()
		v.mu.Lock()
		v.gone = true
		v.mu.Unlock()
		removeViewDir(v)
	}()
	for i := 0; ; i++ { // the debugging port answers once the browser is up
		if c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", l.port), 300*time.Millisecond); err == nil {
			c.Close()
			break
		}
		if i > 100 || v.isGone() {
			a.destroyBrowserView(v)
			return errors.New("the browser did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
	l.view = v
	a.browserNote("browser of %s opened (port %d, %s)", l.key[:min(8, len(l.key))], l.port, map[bool]string{true: "private", false: "kept profile"}[temp])
	a.browserUsed(l)
	go a.watchPage(l, v)
	if l.tab != nil {
		a.emit("browser:open", l.tab.id)
	}
	return nil
}

func (a *App) destroyBrowserView(v *browserView) {
	v.mu.Lock()
	wasGone := v.gone
	v.gone = true
	v.mu.Unlock()
	if !wasGone && v.cmd != nil && v.cmd.Process != nil {
		syscall.Kill(-v.cmd.Process.Pid, syscall.SIGTERM)
		time.AfterFunc(3*time.Second, func() { syscall.Kill(-v.cmd.Process.Pid, syscall.SIGKILL) })
	}
	removeViewDir(v)
}

// removeViewDir deletes a private profile once the browser has let go of it.
func removeViewDir(v *browserView) {
	if !v.temp || v.dir == "" {
		return
	}
	go func() {
		for i := 0; i < 20; i++ {
			if os.RemoveAll(v.dir) == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}()
}

// watchPage reads the page's address and title from the browser itself,
// through its debugging port, for as long as the view lives.
func (a *App) watchPage(l *browserLog, v *browserView) {
	for ; ; time.Sleep(600 * time.Millisecond) {
		if v.isGone() {
			a.browserCloseView(l, "the browser was closed")
			return
		}
		res, err := http.Get(fmt.Sprintf("http://127.0.0.1:%d/json/list", v.port))
		if err != nil {
			continue
		}
		var targets []struct{ Type, URL, Title string }
		json.NewDecoder(res.Body).Decode(&targets)
		res.Body.Close()
		for _, t := range targets {
			if t.Type == "page" {
				v.mu.Lock()
				v.url, v.title = t.URL, t.Title
				v.mu.Unlock()
				break
			}
		}
	}
}

func (a *App) browserTmp() string {
	d := filepath.Join(a.store.browserDir(), "tmp")
	os.MkdirAll(d, 0o755)
	return d
}

// profileBusy: the kept profile can serve one browser at a time.
func (a *App) profileBusy() bool {
	a.browser.mu.Lock()
	defer a.browser.mu.Unlock()
	for _, l := range a.browser.logs {
		l.viewMu.Lock()
		busy := l.view != nil && !l.view.temp
		l.viewMu.Unlock()
		if busy {
			return true
		}
	}
	return false
}

// freePort picks a port for a browser's debugging endpoint.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// closeBrowser removes a chat's browser (its window and, when private, its data).
func (a *App) closeBrowser(key string) {
	a.browserNote("browser %s removed (its chat was closed, restarted or Cmdhive is quitting)", key[:min(8, len(key))])
	a.browser.mu.Lock()
	l := a.browser.logs[key]
	delete(a.browser.logs, key)
	a.browser.mu.Unlock()
	if l == nil {
		return
	}
	l.viewMu.Lock()
	v := l.view
	l.view = nil
	l.viewMu.Unlock()
	if v != nil {
		a.destroyBrowserView(v)
		if l.tab != nil {
			a.emit("browser:gone", l.tab.id)
		}
	}
}

// closeBrowsers is for when Cmdhive quits.
func (a *App) closeBrowsers() {
	a.browser.mu.Lock()
	keys := make([]string, 0, len(a.browser.logs))
	for k := range a.browser.logs {
		keys = append(keys, k)
	}
	a.browser.mu.Unlock()
	for _, k := range keys {
		a.closeBrowser(k)
	}
}

// There is no column to update, and the window is not Cmdhive's to restore.
func (a *App) browserPush(l *browserLog)     {}
func restoreBrowserView(v *browserView) bool { return false }

// browserAttention: the AI is waiting for the user, who gets the chat's card
// and a notification; also bring the browser forward.
func (a *App) browserAttention(l *browserLog) {
	l.viewMu.Lock()
	v := l.view
	l.viewMu.Unlock()
	if v != nil && v.cmd.Process != nil {
		exec.Command("osascript", "-e", fmt.Sprintf(`tell application "System Events" to set frontmost of (first process whose unix id is %d) to true`, v.cmd.Process.Pid)).Run()
	}
}

func (a *App) BrowserTheme(theme string) {
	a.browser.mu.Lock()
	a.browser.theme = theme
	a.browser.mu.Unlock()
}

// BrowserShow brings a chat's browser forward (the chat's globe button).
func (a *App) BrowserShow(tabID int) {
	t := a.tab(tabID)
	if t == nil {
		return
	}
	key, _ := t.browserKey.Load().(string)
	if l := a.browserLogFor(key); l != nil {
		a.browserAttention(l)
	}
}

// BrowserState is a chat's browser as the chat shows it (for tests).
func (a *App) BrowserState(tabID int) map[string]any {
	t := a.tab(tabID)
	if t == nil {
		return nil
	}
	key, _ := t.browserKey.Load().(string)
	l := a.browserLogFor(key)
	if l == nil {
		return nil
	}
	l.viewMu.Lock()
	v := l.view
	l.viewMu.Unlock()
	a.browser.mu.Lock()
	steps := append([]BrowserStep{}, l.steps...)
	a.browser.mu.Unlock()
	out := map[string]any{"steps": steps, "ai": l.ai, "open": v != nil}
	if v != nil {
		v.mu.Lock()
		out["url"], out["title"] = v.url, v.title
		v.mu.Unlock()
	}
	return out
}
