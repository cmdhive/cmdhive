package main

import (
	"bytes"
	"encoding/json"
	"errors"
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

// helperPrompts open the throwaway sessions Cmdhive's own helper runs (titling,
// the enhancer, review, MCP fixing) start. A transcript beginning with one of
// these is a helper, not a conversation, and is kept out of History.
var helperPrompts = []string{
	"Name this conversation with a short, specific title",
	"You improve prompts.",
	"You are a second AI reviewing another AI's work.",
	"You are fixing one MCP server for the user of Cmdhive",
}

func isHelperPrompt(s string) bool {
	s = strings.TrimSpace(s)
	for _, p := range helperPrompts {
		if strings.HasPrefix(s, p) {
			return true
		}
	}
	return false
}

// pruneHelperSessions deletes helper transcripts already written to disk before
// they were cleaned up at the source — so they stop cluttering History and
// taking space. Runs once in the background at startup.
func (a *App) pruneHelperSessions() {
	homes := map[string]bool{}
	for _, ac := range a.store.Config().Accounts {
		home := a.store.Home(ac)
		if home == "" || homes[strings.ToLower(home)] {
			continue
		}
		homes[strings.ToLower(home)] = true
		for _, ch := range ac.provider().Chats(home) {
			if isHelperPrompt(firstUserText(ch.Path)) {
				os.Remove(ch.Path)
				chatCache.Lock()
				delete(chatCache.m, ch.Path)
				chatCache.Unlock()
			}
		}
	}
}

// NameAllChats has the cheapest Claude model (Haiku) read every past
// conversation that has no AI name yet and write it one, in the background. A
// chat already named is skipped, so the pass is cheap to re-run. Progress and
// the finished count come back as events; it stops early on StopNaming.
func (a *App) NameAllChats() error {
	cl := registry["claude"]
	if cl == nil || chatOf(cl) == nil {
		return errors.New("Claude is needed to name chats with Haiku")
	}
	acct, ok := a.store.Pick("claude", "", time.Now())
	if !ok {
		return errors.New("No Claude account has usage left right now")
	}
	if !a.naming.CompareAndSwap(false, true) {
		return errors.New("Already naming chats")
	}
	a.stopNaming.Store(false)

	// Which conversations still need a name (newest copy of each).
	type job struct{ path, key string }
	var jobs []job
	seen := map[string]bool{}
	homes := map[string]bool{}
	for _, ac := range a.store.Config().Accounts {
		home := a.store.Home(ac)
		if home == "" || homes[strings.ToLower(home)] {
			continue
		}
		homes[strings.ToLower(home)] = true
		for _, ch := range ac.provider().Chats(home) {
			key := ch.Provider + "/" + ch.ID
			if seen[key] {
				continue
			}
			seen[key] = true
			if _, done := a.store.Title(key); done {
				continue
			}
			jobs = append(jobs, job{ch.Path, key})
		}
	}

	go func() {
		defer a.naming.Store(false)
		model := ptr(cheapestModel(cl))
		named := 0
		for i, j := range jobs {
			if a.stopNaming.Load() {
				break
			}
			a.emit("names:progress", map[string]any{"done": i, "total": len(jobs)})
			first := firstUserText(j.path)
			if strings.TrimSpace(first) == "" {
				a.store.SetTitle(j.key, "") // nothing to name; don't revisit
				continue
			}
			account := acct
			account.Model = model
			var extra []string
			extra = []string{"--strict-mcp-config"}
			out, err := a.runHelper(cl, account, a.store.home, "review", extra, titleLimit,
				titlePrompt(first, ""), func(string) {}, nil)
			if err != nil {
				if a.stopNaming.Load() {
					break
				}
				continue // leave it unnamed so a later run can retry
			}
			if title := cleanTitleText(out); title != "" {
				a.store.SetTitle(j.key, title)
				named++
				a.emit("history:changed")
			} else {
				a.store.SetTitle(j.key, "")
			}
		}
		a.emit("names:done", map[string]any{"named": named, "total": len(jobs)})
	}()
	return nil
}

// StopNaming asks a running NameAllChats pass to stop after the current chat.
func (a *App) StopNaming() { a.stopNaming.Store(true) }

// nearEmptyMax: a chat bigger than this holds real conversation, so it is never
// "near-empty" and its transcript is not read to check.
const nearEmptyMax = 400 << 10

// EmptyChats lists conversations that got no reply — opened or typed into but
// nothing came back, or abandoned at once. These are the "near-empty" chats the
// cleanup offers to delete; the user confirms the list before any go.
func (a *App) EmptyChats() []ChatView {
	var out []ChatView
	for _, cv := range a.History() {
		if cv.Size > nearEmptyMax {
			continue
		}
		if chatReplies(cv.Ref) == 0 {
			out = append(out, cv)
		}
	}
	return out
}

// DeleteChats removes several conversations at once (the cleanup's "delete
// all"). It keeps going past a file that will not delete and reports the first
// error.
func (a *App) DeleteChats(refs []string) error {
	var firstErr error
	for _, r := range refs {
		if err := a.DeleteChat(r); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// chatReplies counts the assistant's replies near the start of a transcript,
// across Claude and Codex formats — enough to tell an answered chat from an
// abandoned one. A near-empty chat fits entirely in this window.
func chatReplies(path string) int {
	head := headLines(path, 256<<10, 16<<20)
	n := 0
	for _, line := range bytes.Split(head, []byte{'\n'}) {
		switch {
		case bytes.Contains(line, []byte(`"type":"assistant"`)) && !bytes.Contains(line, []byte(`"isApiErrorMessage":true`)):
			n++
		case bytes.Contains(line, []byte(`"agent_message"`)):
			n++
		}
	}
	return n
}

// firstUserText pulls the first thing the user typed from a transcript,
// across Claude and Codex formats, skipping Cmdhive's own bracketed notes.
func firstUserText(path string) string {
	head := headLines(path, 96<<10, 16<<20)
	for _, line := range bytes.Split(head, []byte{'\n'}) {
		var l struct {
			Type    string `json:"type"`
			IsMeta  bool   `json:"isMeta"`
			Message struct {
				Content json.RawMessage `json:"content"`
			} `json:"message"`
			Payload struct {
				Type    string `json:"type"`
				Message string `json:"message"`
				Item    struct {
					Content []struct {
						Text string `json:"text"`
					} `json:"content"`
				} `json:"item"`
			} `json:"payload"`
		}
		if json.Unmarshal(line, &l) != nil {
			continue
		}
		t := ""
		switch {
		case l.Type == "user" && !l.IsMeta:
			t = strings.TrimSpace(contentText(l.Message.Content))
		case l.Payload.Type == "user_message":
			t = strings.TrimSpace(l.Payload.Message)
		case len(l.Payload.Item.Content) > 0:
			t = strings.TrimSpace(l.Payload.Item.Content[0].Text)
		}
		if t != "" && !strings.HasPrefix(t, "<") {
			return t
		}
	}
	return ""
}

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
