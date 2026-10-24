package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// sanitizeClaudeMd must drop every @import so one imported file whose content
// trips Claude's classifier does not fail every new chat Cmdhive starts; the
// rest of the file is kept verbatim.
func TestSanitizeClaudeMdStripsImports(t *testing.T) {
	dir := t.TempDir()
	in := filepath.Join(dir, "CLAUDE.md")
	out := filepath.Join(dir, "out.md")
	body := "# Guidelines\n\nBe concise.\n\n@C:/Users/Kaya/.claude/MAIN.md\n  @./other.md\n\n## Done\n"
	if err := os.WriteFile(in, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := sanitizeClaudeMd(in, out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(out)
	got := string(b)
	if strings.Contains(got, "@C:/Users/Kaya/.claude/MAIN.md") || strings.Contains(got, "@./other.md") {
		t.Fatalf("@import left in output:\n%s", got)
	}
	if !strings.Contains(got, "# Guidelines") || !strings.Contains(got, "Be concise.") || !strings.Contains(got, "## Done") {
		t.Fatalf("non-import content missing:\n%s", got)
	}
	// A line that merely mentions @ in prose (not at the start) is left alone.
	alone := "see @docs for more\n@keep.md\n"
	os.WriteFile(in, []byte(alone), 0o644)
	sanitizeClaudeMd(in, out)
	b, _ = os.ReadFile(out)
	if !strings.Contains(string(b), "see @docs for more") {
		t.Fatalf("prose with @ was wrongly stripped:\n%s", b)
	}
	if strings.Contains(string(b), "@keep.md") {
		t.Fatalf("leading @ import not stripped:\n%s", b)
	}
}
