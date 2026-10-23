package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// A conversation's first useful name comes from the first thing the user types,
// which for a handover is Cmdhive's own takeover prompt — unreadable. So once a
// chat has a real exchange, a throwaway run of the cheapest model reads it and
// writes a short title ("NR3 Development"). It happens once per conversation and
// the result is cached in titles.json, so it never spends usage on the same
// chat twice.

const titleLimit = 45 * time.Second

func (s *Store) titlesPath() string { return filepath.Join(s.root, "titles.json") }

// Title returns the saved AI title for a conversation, if one was written.
func (s *Store) Title(key string) (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	t, ok := s.titles[key]
	return t, ok && t != ""
}

// SetTitle records a conversation's AI title. An empty string marks the
// conversation as "tried" so a failed naming is not retried forever.
func (s *Store) SetTitle(key, title string) {
	s.mu.Lock()
	if s.titles == nil {
		s.titles = map[string]string{}
	}
	s.titles[key] = title
	b, _ := json.MarshalIndent(s.titles, "", "  ")
	s.mu.Unlock()
	os.WriteFile(s.titlesPath(), b, 0o644)
}

func titleKey(providerID, sessionID string) string { return providerID + "/" + sessionID }

// cheapestModel is the model to name a chat with: the smallest, cheapest one
// the AI offers, so titling costs as little as possible. Empty means "use the
// account's default" (when nothing cheaper can be identified).
func cheapestModel(p Provider) string {
	switch p.ID() {
	case "claude":
		for _, m := range p.Models() {
			if strings.EqualFold(m.Family, "Haiku") || strings.Contains(strings.ToLower(m.ID), "haiku") {
				return m.ID
			}
		}
		return "haiku"
	case "codex":
		for _, m := range p.Models() {
			s := strings.ToLower(m.ID + " " + m.Name)
			if strings.Contains(s, "nano") || strings.Contains(s, "mini") {
				return m.ID
			}
		}
	case "gemini":
		for _, m := range p.Models() {
			if strings.Contains(strings.ToLower(m.ID+" "+m.Name), "flash") {
				return m.ID
			}
		}
	}
	return ""
}

// maybeTitle generates and saves a chat's title in the background, unless one
// was already written (or attempted). first is the user's opening request and
// reply is what the agent answered — enough to name the topic without opening
// the whole transcript.
func (a *App) maybeTitle(t *Tab, first, reply string) {
	t.mu.Lock()
	p, acct, cwd, sess := t.agent, t.acct, t.cwd, t.sessionID
	t.mu.Unlock()
	if p == nil || sess == "" || strings.TrimSpace(first) == "" {
		return
	}
	key := titleKey(p.ID(), sess)
	if _, done := a.store.Title(key); done {
		return
	}
	if !t.titling.CompareAndSwap(false, true) {
		return // a naming is already in flight for this tab
	}
	go func() {
		defer t.titling.Store(false)
		account, ok := a.store.Account(acct)
		if !ok {
			return
		}
		account.Model = ptr(cheapestModel(p))
		var extra []string
		if p.ID() == "claude" {
			extra = []string{"--strict-mcp-config"}
		}
		out, err := a.runHelper(p, account, cwd, "review", extra, titleLimit,
			titlePrompt(first, reply), func(string) {}, nil)
		if err != nil {
			a.store.SetTitle(key, "") // do not keep retrying a chat that will not name
			return
		}
		title := cleanTitleText(out)
		if title == "" {
			a.store.SetTitle(key, "")
			return
		}
		a.store.SetTitle(key, title)
		a.emit("history:changed")
	}()
}

func titlePrompt(first, reply string) string {
	first = clip(strings.TrimSpace(first), 1500)
	reply = clip(strings.TrimSpace(reply), 800)
	var b strings.Builder
	b.WriteString("Name this conversation with a short, specific title for a history list. " +
		"2 to 4 words, Title Case, no quotes, no punctuation at the end, no preamble. " +
		"Name the actual project or topic (for example \"NR3 Console Rework\" or \"Shop API Login Bug\"), not a generic phrase.\n\n" +
		"First request:\n" + first + "\n")
	if reply != "" {
		b.WriteString("\nThe assistant's reply began:\n" + reply + "\n")
	}
	b.WriteString("\nReply with the title only.")
	return b.String()
}

// cleanTitleText takes the model's answer down to a bare title: no quotes,
// no fences, no trailing period, one line, a sane length.
func cleanTitleText(s string) string {
	s = strings.TrimSpace(s)
	if m := enhanceFence.FindStringSubmatch(s); m != nil {
		s = strings.TrimSpace(m[1])
	}
	if i := strings.IndexAny(s, "\r\n"); i >= 0 {
		s = s[:i]
	}
	s = strings.TrimSpace(s)
	s = strings.Trim(s, `"'“”`)
	s = strings.TrimRight(s, ".。!! ")
	if len([]rune(s)) > 60 {
		s = cutText(s, 60)
	}
	return strings.TrimSpace(s)
}

func ptr[T any](v T) *T { return &v }

// clip is cutText that is safe when s is already shorter than n bytes.
func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return cutText(s, n)
}
