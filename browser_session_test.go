package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// A chat with a browser log and somewhere to see the cards it raises.
func sessionApp(t *testing.T) (*App, *browserLog, chan Ev) {
	t.Helper()
	store, err := newStoreAt(t.TempDir(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app := NewApp(store)
	evs := make(chan Ev, 32)
	app.emit = func(event string, data ...any) {
		if event == "chat:ev" {
			for _, e := range data[1].([]Ev) {
				select {
				case evs <- e:
				default:
				}
			}
		}
	}
	tab := &Tab{id: 1, agent: registry["claude"], profile: "claude"}
	tab.adopted.Store(true)
	app.tabs[1] = tab
	l := &browserLog{key: "k", tab: tab, ai: "Claude", allowed: map[string]bool{}}
	app.browser.logs = map[string]*browserLog{"k": l}
	return app, l, evs
}

// waitAsk returns the request id of the next card the user is shown.
func waitAsk(t *testing.T, evs chan Ev, tool string) Ev {
	t.Helper()
	for {
		select {
		case e := <-evs:
			if e["k"] == "ask" && e["tool"] == tool {
				return e
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("no %s card was shown", tool)
			return nil
		}
	}
}

// Before anything is opened, the user is asked which browser this chat gets.
// Saying yes to staying signed in uses the one profile that is kept.
func TestBrowserAsksWhichBrowser(t *testing.T) {
	app, l, evs := sessionApp(t)
	got := make(chan string, 1)
	go func() { got <- app.browserChooseMode(context.Background(), l) }()

	ask := waitAsk(t, evs, "BrowserSession")
	if !strings.Contains(ask["desc"].(string), "kept on this PC") {
		t.Errorf("the warning does not say where the data is kept: %v", ask["desc"])
	}
	labels := ask["labels"].(map[string]any)
	if labels["allow"] != "Stay signed in" || labels["deny"] != "Private" {
		t.Errorf("buttons: %v", labels)
	}
	if err := app.ChatAnswer(1, ask["req"].(string), "allow"); err != nil {
		t.Fatal(err)
	}
	if mode := <-got; mode != browserKeep {
		t.Fatalf("mode %q, want %q", mode, browserKeep)
	}
	// Asked once: the answer holds for the rest of the chat, however often the
	// window is closed and opened again.
	if mode := app.browserChooseMode(context.Background(), l); mode != browserKeep {
		t.Errorf("second open: %q", mode)
	}
	select {
	case e := <-evs:
		if e["k"] == "ask" && e["tool"] == "BrowserSession" {
			t.Error("the user was asked twice in one chat")
		}
	default:
	}
}

// Saying no, and never answering, both mean the browser that keeps nothing.
func TestBrowserPrivateIsTheDefault(t *testing.T) {
	app, l, evs := sessionApp(t)
	got := make(chan string, 1)
	go func() { got <- app.browserChooseMode(context.Background(), l) }()
	ask := waitAsk(t, evs, "BrowserSession")
	app.ChatAnswer(1, ask["req"].(string), "deny")
	if mode := <-got; mode != browserPrivate {
		t.Fatalf("mode %q, want %q", mode, browserPrivate)
	}

	// The chat ends while the card is still open: nothing is kept.
	app2, l2, evs2 := sessionApp(t)
	ctx, cancel := context.WithCancel(context.Background())
	got2 := make(chan string, 1)
	go func() { got2 <- app2.browserChooseMode(ctx, l2) }()
	waitAsk(t, evs2, "BrowserSession")
	cancel()
	select {
	case mode := <-got2:
		if mode != browserPrivate {
			t.Errorf("unanswered: %q, want %q", mode, browserPrivate)
		}
	case <-time.After(3 * time.Second):
		t.Error("an unanswered question never gave up")
	}
}

// An account running out must not cost the user their browser: the window, its
// logins and its steps belong to the chat, not to the account that opened it.
func TestBrowserSurvivesAccountSwitch(t *testing.T) {
	app, l, _ := sessionApp(t)
	tab := l.tab
	tab.browserKey.Store("k")
	l.port = 45123
	l.mode = browserKeep
	app.browserLogStep(l, "do", "Opened example.com")

	os.MkdirAll(filepath.Dir(app.store.browserCLI()), 0o755)
	os.WriteFile(app.store.browserCLI(), nil, 0o644) // browserReady: as if Playwright were installed
	if !app.store.browserReady() {
		t.Skip("node is not installed on this PC")
	}

	srv := app.browserMcp(tab, registry["codex"]) // the next account, and another AI
	if srv == nil {
		t.Fatal("no browser for the relaunched chat")
	}
	if key, _ := tab.browserKey.Load().(string); key != "k" {
		t.Fatalf("the chat was given a different browser: %q", key)
	}
	if app.browserLogFor("k") == nil {
		t.Fatal("the browser was thrown away by the switch")
	}
	args := strings.Join(srv.Args, " ")
	if !strings.Contains(args, "--key k") {
		t.Errorf("the new agent was not handed the same browser: %v", srv.Args)
	}
	app.browser.mu.Lock()
	steps, ai, mode, port := len(l.steps), l.ai, l.mode, l.port
	app.browser.mu.Unlock()
	if steps != 1 {
		t.Errorf("its steps were lost: %d", steps)
	}
	if mode != browserKeep {
		t.Errorf("the user is asked again after a switch: mode %q", mode)
	}
	if port != 45123 {
		t.Errorf("port changed to %d, so the old window is orphaned", port)
	}
	if ai != "ChatGPT" {
		t.Errorf("the column still names the old AI: %q", ai)
	}
}

// Deleting the signed-in browser's data removes the one profile it keeps.
func TestDeleteBrowserData(t *testing.T) {
	app, _, _ := sessionApp(t)
	profile := app.store.browserProfileDir()
	os.MkdirAll(filepath.Join(profile, "Default"), 0o755)
	os.WriteFile(filepath.Join(profile, "Default", "Cookies"), []byte("signed in"), 0o644)
	if err := app.BrowserClearData(); err != nil {
		t.Fatal(err)
	}
	if fileExists(profile) {
		t.Error("the profile is still there")
	}

	// And the same on the way out, when Settings asks for it.
	os.MkdirAll(profile, 0o755)
	app.forgetBrowserOnExit()
	if !fileExists(profile) {
		t.Error("it was deleted without being asked for")
	}
	c := app.store.Config()
	c.BrowserForget = true
	app.store.saveConfig(c)
	app.forgetBrowserOnExit()
	if fileExists(profile) {
		t.Error("it outlived a close with the setting on")
	}
}
