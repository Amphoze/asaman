package meta

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
)

// GATE: tag replay is accumulative ordered-set; fav/note are replacement.
func TestFoldSemantics(t *testing.T) {
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	appendR := func(field, op, val string) {
		if _, err := Append(store, Record{Actor: "cli", Key: "s1", Field: field, Op: op, Value: val}); err != nil {
			t.Fatal(err)
		}
	}
	appendR("tag", "add", "alpha")
	appendR("tag", "add", "beta")
	appendR("tag", "remove", "alpha")
	appendR("fav", "set", "1")
	appendR("fav", "set", "0")
	appendR("note", "set", "hello")
	appendR("note", "clear", "")

	st, err := Fold(store)
	if err != nil {
		t.Fatal(err)
	}
	c := st["s1"]
	if len(c.Tags) != 1 || c.Tags[0] != "beta" {
		t.Errorf("tags = %v want [beta]", c.Tags)
	}
	if c.Fav {
		t.Error("fav should be false (last-writer-wins)")
	}
	if c.Note != "" {
		t.Errorf("note = %q want empty", c.Note)
	}
}

// GATE: a truncated partial tail must be repaired before the next append, and
// the two new actions must both survive.
func TestRepairBeforeAppend(t *testing.T) {
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	if _, err := Append(store, Record{Actor: "cli", Key: "s1", Field: "fav", Op: "set", Value: "1"}); err != nil {
		t.Fatal(err)
	}
	// Simulate a crash mid-append: a partial JSON line with no trailing newline.
	f, _ := os.OpenFile(store, os.O_WRONLY|os.O_APPEND, 0o644)
	f.WriteString(`{"action_id":99,"key":"s1","field":"tag","op":"add","va`)
	f.Close()

	if _, err := Append(store, Record{Actor: "cli", Key: "s1", Field: "tag", Op: "add", Value: "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Append(store, Record{Actor: "cli", Key: "s1", Field: "tag", Op: "add", Value: "y"}); err != nil {
		t.Fatal(err)
	}

	st, err := Fold(store)
	if err != nil {
		t.Fatal(err)
	}
	c := st["s1"]
	if !c.Fav {
		t.Error("original fav lost")
	}
	if len(c.Tags) != 2 || c.Tags[0] != "x" || c.Tags[1] != "y" {
		t.Errorf("appended tags after repair = %v want [x y]", c.Tags)
	}
	// partial bytes quarantined
	if _, err := os.Stat(store + ".corrupt"); err != nil {
		t.Errorf("partial tail not quarantined: %v", err)
	}
}

// GATE: concurrent writers serialize; no lost/interleaved records.
func TestConcurrentAppends(t *testing.T) {
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	const n = 50
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			Append(store, Record{Actor: "cli", Key: "s1", Field: "tag", Op: "add", Value: tagName(i)})
		}(i)
	}
	wg.Wait()
	st, err := Fold(store)
	if err != nil {
		t.Fatal(err)
	}
	if len(st["s1"].Tags) != n {
		t.Errorf("got %d tags, want %d (lost/interleaved records)", len(st["s1"].Tags), n)
	}
}

func tagName(i int) string { return "t" + string(rune('A'+i%26)) + string(rune('0'+i/26)) }
