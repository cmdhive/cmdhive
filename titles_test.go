package main

import (
	"os"
	"path/filepath"
	"testing"
)

// A helper run's transcript opens with a helper prompt and must be recognised
// as one from its first user message, so it is kept out of History and pruned.
func TestFirstUserTextAndHelperDetection(t *testing.T) {
	dir := t.TempDir()
	helper := filepath.Join(dir, "helper.jsonl")
	os.WriteFile(helper, []byte(
		`{"type":"user","message":{"content":"Name this conversation with a short, specific title for a history list. 2 to 4 words, Title Case."}}`+"\n"+
			`{"type":"assistant","message":{"content":[{"type":"text","text":"Shop API"}]}}`+"\n"), 0o644)
	real := filepath.Join(dir, "real.jsonl")
	os.WriteFile(real, []byte(
		`{"type":"user","message":{"content":[{"type":"text","text":"fix the login bug in the shop api"}]}}`+"\n"), 0o644)

	if got := firstUserText(helper); !isHelperPrompt(got) {
		t.Fatalf("helper not detected; first user text = %q", got)
	}
	if got := firstUserText(real); isHelperPrompt(got) {
		t.Fatalf("real chat wrongly flagged as helper; first user text = %q", got)
	}
}
