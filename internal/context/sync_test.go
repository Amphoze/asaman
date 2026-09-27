package context

import (
	"os"
	"path/filepath"
	"testing"
)

// GATE: writing through the CLAUDE.md symlink must land on the real AGENTS.md
// and leave CLAUDE.md a symlink; both paths must read byte-identical after.
func TestSyncIndexPreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "AGENTS.md")
	link := filepath.Join(dir, "CLAUDE.md")
	lock := filepath.Join(dir, ".lock")
	markers := [2]string{"<!-- asaman:index -->", "<!-- /asaman:index -->"}

	orig := "# Rules\n\nBe careful.\n\n" + markers[0] + "\nOLD INDEX\n" + markers[1] + "\n\n## Tail\nkeep me\n"
	if err := os.WriteFile(real, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("AGENTS.md", link); err != nil {
		t.Fatal(err)
	}

	if err := SyncIndex(link, "## Feedback\n- **x** — y", markers, lock); err != nil {
		t.Fatalf("SyncIndex: %v", err)
	}

	// CLAUDE.md still a symlink
	fi, err := os.Lstat(link)
	if err != nil || fi.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("CLAUDE.md is no longer a symlink (mode=%v err=%v)", fi.Mode(), err)
	}
	rb, _ := os.ReadFile(real)
	lb, _ := os.ReadFile(link)
	if string(rb) != string(lb) {
		t.Error("real and symlink content differ")
	}
	got := string(rb)
	if !contains(got, "## Feedback") || contains(got, "OLD INDEX") {
		t.Errorf("index not replaced:\n%s", got)
	}
	// content outside markers preserved
	if !contains(got, "Be careful.") || !contains(got, "keep me") {
		t.Errorf("content outside markers lost:\n%s", got)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
