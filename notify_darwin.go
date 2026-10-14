//go:build darwin

package main

import (
	"os/exec"
	"strings"
)

// notifyBackground shows a notification unless AIT is in front or the user
// turned notifications off.
func (a *App) notifyBackground(title, text string) {
	if !a.store.Config().notify() || inFront() {
		return
	}
	dockBounce()
	showNotification(title, text)
}

// showNotification shows a macOS notification (through osascript, so it needs
// no signed app; it is listed under Script Editor in System Settings).
func showNotification(title, text string) bool {
	q := func(s string) string {
		s = strings.ToValidUTF8(s, "")
		return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
	}
	return exec.Command("osascript", "-e", "display notification "+q(text)+" with title "+q(title)).Run() == nil
}

// notifyTab tells the user about a chat tab's turn: "<AI> finished" with the
// start of the reply, or "<AI> stopped" with the error. Not for a standby
// tab or the preparation turn of a handover.
func (a *App) notifyTab(t *Tab, done, failed, text, errText string) {
	t.mu.Lock()
	skip := t.reading || t.closed || !t.adopted.Load()
	name := t.agent.Name()
	t.mu.Unlock()
	if skip {
		return
	}
	title := name + done
	if errText != "" && failed != "" {
		title, text = name+failed, errText
	}
	text = strings.Join(strings.Fields(text), " ")
	if len(text) > 180 {
		text = cutText(text, 180) + "…"
	}
	if text == "" {
		text = "Open AIT to see the result."
	}
	a.notifiedTab.Store(int64(t.id))
	a.notifyBackground(title, text)
}
