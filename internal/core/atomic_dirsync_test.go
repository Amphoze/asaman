package core

import (
	"os"
	"path/filepath"
	"testing"
)

// F2: AtomicWrite completes (incl. parent-dir fsync path) and content is correct.
func TestAtomicWriteContentAndDirSync(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "sub", "file.txt")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := AtomicWrite(p, []byte("hello")); err != nil {
		t.Fatalf("AtomicWrite: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hello" {
		t.Errorf("content = %q want hello", got)
	}
	// fsyncDir on a real directory must succeed.
	if err := fsyncDir(filepath.Dir(p)); err != nil {
		t.Errorf("fsyncDir real dir: %v", err)
	}
}
