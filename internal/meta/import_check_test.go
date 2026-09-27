package meta

import (
	"database/sql"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

func makeArchiveFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "csess.db")
	db, _ := sql.Open("sqlite", p)
	defer db.Close()
	db.Exec(`CREATE TABLE meta(session_id TEXT PRIMARY KEY, fav INT DEFAULT 0, tags TEXT DEFAULT '', note TEXT DEFAULT '', updated TEXT)`)
	// A non-dated archive id whose csess-join derivation the adapter would NOT
	// reproduce as a real session — must be flagged, not silently orphaned.
	db.Exec(`INSERT INTO meta VALUES(?,?,?,?,?)`, "reports--old--thing", 1, "x", "", "t")
	return p
}

// F4: keys with no corresponding session ID are flagged Unmatched, not folded in.
func TestImportCsessCrossCheck(t *testing.T) {
	csess := makeArchiveFixture(t)
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	known := map[string]bool{"claude:aaaa": true, "archive:2026-09-27--real": true}

	rep, err := ImportCsessChecked(csess, store, known)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Unmatched != 1 || rep.Mapped != 0 {
		t.Fatalf("mapped=%d unmatched=%d details=%v want mapped=0 unmatched=1",
			rep.Mapped, rep.Unmatched, rep.Details)
	}
	// The unmatched key must NOT have been written into the store.
	st, _ := Fold(store)
	if _, ok := st["archive:reports--old--thing"]; ok {
		t.Errorf("orphan key was folded in despite no matching session")
	}
}

// F4: with the key present in the known set, it imports normally.
func TestImportCsessCrossCheckMatches(t *testing.T) {
	csess := makeArchiveFixture(t)
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	known := map[string]bool{"archive:reports--old--thing": true}
	rep, err := ImportCsessChecked(csess, store, known)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mapped != 1 || rep.Unmatched != 0 {
		t.Fatalf("mapped=%d unmatched=%d want mapped=1 unmatched=0", rep.Mapped, rep.Unmatched)
	}
}
