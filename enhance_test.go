package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The enhancer runs as its own AI, with the shared memory's index, and the
// composer gets back only the rewritten prompt: not the AI's narration, not
// the fence it wraps it in.
func TestEnhancePromptRoundTrip(t *testing.T) {
	root, home, cwd := tempRoot(t), t.TempDir(), t.TempDir()
	os.MkdirAll(filepath.Join(home, ".claude"), 0o755)
	os.WriteFile(filepath.Join(home, ".claude", ".credentials.json"), []byte("{}"), 0o644)
	mem := t.TempDir()
	os.WriteFile(filepath.Join(mem, memoryIndex), []byte("# Memory\n- [MYC2](myc2.md) — the Roblox nation game\n"), 0o644)
	b, _ := json.Marshal(Config{Accounts: []Account{{ID: "main", Label: "Main"}}, StartingDir: cwd, MemoryDir: mem})
	os.MkdirAll(root, 0o755)
	os.WriteFile(filepath.Join(root, "config.json"), b, 0o644)
	t.Setenv("AIT_FAKE_CLI", "1")
	t.Setenv("AIT_FAKE_HOME", home)
	store, _ := newStoreAt(root, home)
	old := registry["claude"]
	registry["claude"] = fakeChat{old.(*claude)}
	defer func() { registry["claude"] = old }()
	app := NewApp(store)
	app.emit = func(string, ...any) {}
	app.tabs[1] = &Tab{id: 1, agent: registry["claude"], profile: "claude", cwd: cwd}

	got, err := app.EnhancePrompt(1, "fix the shop bug in myc2")
	if err != nil {
		t.Fatal(err)
	}
	if got != "ENHANCED with memory=true" {
		t.Fatalf("composer would get %q", got)
	}
}

// Too little to work with, or so much it speaks for itself: no AI is started.
func TestEnhancePromptRefusesUnusableDrafts(t *testing.T) {
	store, _ := newStoreAt(t.TempDir(), t.TempDir())
	app := NewApp(store)
	app.tabs[1] = &Tab{id: 1, agent: registry["claude"], profile: "claude"}
	for _, draft := range []string{"", "   ", "fix", "fixitall!", strings.Repeat("word ", 2000)} {
		if _, err := app.EnhancePrompt(1, draft); err == nil {
			t.Errorf("enhanced %q", shorten(draft, 30))
		}
	}
	if _, err := app.EnhancePrompt(99, "a real prompt here"); err == nil {
		t.Error("enhanced for a chat that does not exist")
	}
}

func TestCleanEnhanced(t *testing.T) {
	for in, want := range map[string]string{
		"Refactor auth.ts":                     "Refactor auth.ts",
		"```\nRefactor auth.ts\n```":           "Refactor auth.ts",
		"```markdown\nRefactor auth.ts\n```":   "Refactor auth.ts",
		"\"Refactor auth.ts\"":                 "Refactor auth.ts",
		"“Refactor auth.ts”":                   "Refactor auth.ts",
		"<draft>\nRefactor auth.ts\n</draft>":  "Refactor auth.ts",
		"Keep \"quotes\" inside the prompt":    "Keep \"quotes\" inside the prompt",
		"\"Rename\" it to \"Shop\"":            "\"Rename\" it to \"Shop\"", // starts and ends with a quotation, not wrapped in one
		"  \n\n Refactor auth.ts \n ":          "Refactor auth.ts",
		"Step one:\n```go\nx := 1\n```\nDone.": "Step one:\n```go\nx := 1\n```\nDone.", // code inside the prompt stays
	} {
		if got := cleanEnhanced(in); got != want {
			t.Errorf("cleanEnhanced(%q) = %q, want %q", in, got, want)
		}
	}
}

// The enhancer is told the user's projects and the draft, and not to do the work.
func TestEnhancePromptText(t *testing.T) {
	p := enhancePrompt("fix the shop", `C:\work`, "- [MYC2](myc2.md) — nation game")
	for _, want := range []string{"<draft>\nfix the shop\n</draft>", "<memory>", "MYC2", "Do not answer the request", "Reply with the improved prompt only"} {
		if !strings.Contains(p, want) {
			t.Errorf("prompt lacks %q", want)
		}
	}
	if strings.Contains(enhancePrompt("fix the shop", `C:\work`, ""), "<memory>") {
		t.Error("an empty memory is still announced")
	}
}
