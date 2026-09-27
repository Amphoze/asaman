package sessions

import (
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/core"
)

// countingAdapter parses real files from disk and counts ParseFile calls/path.
type countingAdapter struct {
	dir    string
	mu     sync.Mutex
	parsed map[string]int
}

func (c *countingAdapter) Name() string                    { return "count" }
func (c *countingAdapter) TimeLog() []string               { return nil }
func (c *countingAdapter) ContextPath() string             { return "" }
func (c *countingAdapter) MemoryMode() adapters.MemoryMode { return adapters.ModeNone }
func (c *countingAdapter) SyncIndex(string) error          { return nil }
func (c *countingAdapter) Files() []string                 { return core.ExpandGlob(filepath.Join(c.dir, "*.txt")) }
func (c *countingAdapter) ParseFile(p string) ([]adapters.SessionRef, error) {
	c.mu.Lock()
	c.parsed[p]++
	c.mu.Unlock()
	b, _ := os.ReadFile(p)
	id := "count:" + filepath.Base(p)
	return []adapters.SessionRef{{ID: id, Agent: "count", Kind: "session",
		Title: filepath.Base(p), Path: p,
		Events: []adapters.Event{{Idx: 0, Role: "user", Text: string(b)}}}}, nil
}
func (c *countingAdapter) Sessions() ([]adapters.SessionRef, error) {
	var out []adapters.SessionRef
	for _, p := range c.Files() {
		r, _ := c.ParseFile(p)
		out = append(out, r...)
	}
	return out, nil
}

// F3: reindex is incremental — unchanged files are not re-parsed; an edit
// re-parses only that file; a delete removes its rows.
func TestReindexIncremental(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("alpha content"), 0o644)
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bravo content"), 0o644)
	db, _ := core.Open(filepath.Join(dir, "c.db"))
	defer db.Close()
	db.InitSchema()
	ad := &countingAdapter{dir: dir, parsed: map[string]int{}}

	if err := Reindex(db, []adapters.Adapter{ad}, nil); err != nil {
		t.Fatal(err)
	}
	// first pass parses both
	if ad.parsed[filepath.Join(dir, "a.txt")] != 1 || ad.parsed[filepath.Join(dir, "b.txt")] != 1 {
		t.Fatalf("first pass parse counts wrong: %v", ad.parsed)
	}

	// second pass, nothing changed → no re-parse
	if err := Reindex(db, []adapters.Adapter{ad}, nil); err != nil {
		t.Fatal(err)
	}
	if ad.parsed[filepath.Join(dir, "a.txt")] != 1 {
		t.Errorf("unchanged file re-parsed: %v", ad.parsed)
	}

	// edit b → only b re-parsed
	os.WriteFile(filepath.Join(dir, "b.txt"), []byte("bravo EDITED"), 0o644)
	if err := Reindex(db, []adapters.Adapter{ad}, nil); err != nil {
		t.Fatal(err)
	}
	if ad.parsed[filepath.Join(dir, "a.txt")] != 1 {
		t.Errorf("a re-parsed on b edit: %v", ad.parsed)
	}
	if ad.parsed[filepath.Join(dir, "b.txt")] != 2 {
		t.Errorf("b not re-parsed on edit: %v", ad.parsed)
	}
	hits, _ := Search(db, "EDITED", Opts{})
	if len(hits) == 0 {
		t.Error("edited content not searchable after incremental reindex")
	}

	// delete a → its rows gone
	os.Remove(filepath.Join(dir, "a.txt"))
	if err := Reindex(db, []adapters.Adapter{ad}, nil); err != nil {
		t.Fatal(err)
	}
	hits, _ = Search(db, "alpha", Opts{})
	if len(hits) != 0 {
		t.Errorf("deleted session still searchable: %v", hits)
	}
}
