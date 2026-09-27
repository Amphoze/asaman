package memory

import (
	"path/filepath"
	"testing"
)

func load(t *testing.T) map[string]Fact {
	t.Helper()
	facts, err := LoadDir(filepath.Join("..", "..", "testdata", "memory"))
	if err != nil {
		t.Fatalf("LoadDir: %v", err)
	}
	m := map[string]Fact{}
	for _, f := range facts {
		m[f.Name] = f
	}
	return m
}

func TestLoadDirSkipsIndex(t *testing.T) {
	m := load(t)
	if _, ok := m["MEMORY"]; ok {
		t.Error("MEMORY.md must be skipped")
	}
	if len(m) != 4 {
		t.Fatalf("want 4 facts, got %d (%v)", len(m), keys(m))
	}
}

func TestPlainFrontmatter(t *testing.T) {
	f := load(t)["never_guess"]
	if f.Type != "feedback" || f.Category != "Feedback" {
		t.Errorf("type/category = %q/%q", f.Type, f.Category)
	}
	if f.Origin["agent"] != "claude" || f.Origin["date"] != "2026-04-03" {
		t.Errorf("origin = %v", f.Origin)
	}
	if !contains(f.Body, "Never guess") {
		t.Errorf("body missing: %q", f.Body)
	}
}

func TestNestedMetadataType(t *testing.T) {
	f := load(t)["feedback-destructive-commands"]
	if f.Type != "feedback" {
		t.Errorf("nested type not resolved: %q", f.Type)
	}
}

func TestSupersession(t *testing.T) {
	f := load(t)["old_plan"]
	if !f.Superseded() || f.SupersededBy[0] != "new_plan" {
		t.Errorf("supersession not parsed: %+v", f.SupersededBy)
	}
}

func keys(m map[string]Fact) []string {
	var k []string
	for x := range m {
		k = append(k, x)
	}
	return k
}
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
