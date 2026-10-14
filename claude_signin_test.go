package main

import (
	"os"
	"path/filepath"
	"testing"
)

// On a Mac Claude Code keeps its login in the Keychain, so there is no
// .credentials.json; the account in claude's state file still shows the
// user is signed in, and AIT must create their account from it.
func TestClaudeSignedInWithoutCredentialsFile(t *testing.T) {
	c := &claude{userHome: t.TempDir()}
	extra := t.TempDir()
	if c.SignedIn(extra) {
		t.Fatal("an empty folder counts as signed in")
	}
	os.WriteFile(filepath.Join(extra, ".claude.json"), []byte(`{"oauthAccount":{"emailAddress":"a@b.c"}}`), 0o644)
	if !c.SignedIn(extra) {
		t.Fatal("a folder with a recorded account is not signed in")
	}
}
