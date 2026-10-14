//go:build windows

package main

import (
	"encoding/json"
	"testing"
	"time"
)

// The column is sent the page, the steps in order with their numbers, and the
// questions waiting, oldest first, each with the right wording and buttons.
func TestBrowserColumnState(t *testing.T) {
	store, _ := newStoreAt(t.TempDir(), t.TempDir())
	app := NewApp(store)
	tab := &Tab{id: 3, agent: registry["claude"], profile: "claude"}
	tab.working.Store(true)
	l := &browserLog{key: "k", tab: tab, ai: "Claude", allowed: map[string]bool{}}
	app.browser.logs = map[string]*browserLog{"k": l}
	app.browserLogStep(l, "plan", "I'll open the docs.")
	app.browserLogStep(l, "do", "Open example.com")
	app.browser.mu.Lock()
	app.browser.asks = map[string]*browserAsk{
		"b": {n: 2, key: "k", help: true, message: "Solve the CAPTCHA"},
		"a": {n: 1, key: "k", host: "example.org"},
		"x": {n: 3, key: "other", host: "elsewhere.com"},
	}
	app.browser.theme = `{"vars":{"--accent":"#d97757"},"theme":"campbell"}`
	app.browser.mu.Unlock()
	v := &browserView{key: "k", url: "https://example.com/", title: "Example Domain"}
	var s struct {
		AI    string
		URL   string
		Title string
		Busy  bool
		Keep  bool
		Steps []BrowserStep
		Asks  []struct {
			Req    string
			Help   bool
			Text   string
			Always bool
			Labels map[string]string
		}
		Theme struct {
			Vars  map[string]string
			Theme string
		}
	}
	if err := json.Unmarshal(app.browserStateJSON(l, v), &s); err != nil {
		t.Fatal(err)
	}
	if s.AI != "Claude" || s.URL != "https://example.com/" || s.Title != "Example Domain" || !s.Busy || s.Keep {
		t.Errorf("page: %+v", s)
	}
	if len(s.Steps) != 2 || s.Steps[0].Seq != 1 || s.Steps[1].Seq != 2 || s.Steps[1].Text != "Open example.com" {
		t.Errorf("steps: %+v", s.Steps)
	}
	if len(s.Asks) != 2 || s.Asks[0].Req != "a" || s.Asks[0].Text != "example.org" || !s.Asks[0].Always || s.Asks[0].Help {
		t.Fatalf("site question: %+v", s.Asks)
	}
	if h := s.Asks[1]; h.Req != "b" || !h.Help || h.Text != "Solve the CAPTCHA" || h.Always || h.Labels["allow"] != "I'm done" || h.Labels["deny"] != "Can't do it" {
		t.Errorf("help question: %+v", h)
	}
	if s.Theme.Vars["--accent"] != "#d97757" || s.Theme.Theme != "campbell" {
		t.Errorf("theme: %+v", s.Theme)
	}
	// Answers from the column only reach this browser's own questions.
	got := make(chan string, 1)
	app.browser.mu.Lock()
	app.browser.asks["a"].answer = got
	app.browser.mu.Unlock()
	app.browserColumnMessage(&browserLog{key: "other"}, v, "answer:a|allow")
	select {
	case <-got:
		t.Error("another browser's column answered it")
	case <-time.After(150 * time.Millisecond):
	}
	app.browserColumnMessage(l, v, "answer:a|always")
	select {
	case d := <-got:
		if d != "always" {
			t.Errorf("decision %q", d)
		}
	case <-time.After(2 * time.Second):
		t.Error("the column's answer did not arrive")
	}
}
