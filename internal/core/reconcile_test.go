package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileDetectsAllChanges(t *testing.T) {
	dir := t.TempDir()
	a := filepath.Join(dir, "a.md")
	b := filepath.Join(dir, "sub", "b.md")
	c := filepath.Join(dir, "c.md")
	write(t, a, "alpha")
	write(t, b, "bravo")
	write(t, c, "charlie")

	glob := []string{filepath.Join(dir, "**", "*.md")}
	old, _ := ScanInventory(glob)
	if len(old) != 3 {
		t.Fatalf("expected 3 files, got %d", len(old))
	}

	// edit a (same length "alpha"->"ALPHA" keeps size 5 → mtime-agnostic detection)
	write(t, a, "ALPHA")
	// delete c
	os.Remove(c)
	// add d with an OLDER mtime than the originals
	d := filepath.Join(dir, "d.md")
	write(t, d, "delta")
	old2 := time.Now().Add(-72 * time.Hour)
	os.Chtimes(d, old2, old2)

	cur, _ := ScanInventory(glob)
	ch := Diff(old, cur)

	if len(ch.Edited) != 1 || filepath.Base(ch.Edited[0]) != "a.md" {
		t.Errorf("edited = %v", ch.Edited)
	}
	if len(ch.Deleted) != 1 || filepath.Base(ch.Deleted[0]) != "c.md" {
		t.Errorf("deleted = %v", ch.Deleted)
	}
	if len(ch.Added) != 1 || filepath.Base(ch.Added[0]) != "d.md" {
		t.Errorf("added (incl older-mtime) = %v", ch.Added)
	}
	if !ch.Any() {
		t.Error("Any() should be true")
	}
}
