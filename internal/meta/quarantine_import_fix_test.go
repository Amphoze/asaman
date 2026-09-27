package meta

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// A partial (crash) tail must never be destroyed unless it has been durably
// quarantined first. If quarantine cannot be written, Append must fail and
// leave the store byte-for-byte intact.
func TestAppendFailsWhenQuarantineUnavailable(t *testing.T) {
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	partial := []byte(`{"action_id":99`) // no trailing newline => incomplete tail
	if err := os.WriteFile(store, partial, 0o600); err != nil {
		t.Fatal(err)
	}
	// Make the quarantine path unwritable as a file by occupying it with a dir.
	if err := os.Mkdir(store+".corrupt", 0o700); err != nil {
		t.Fatal(err)
	}

	_, err := Append(store, Record{Key: "s", Field: "note", Op: "set", Value: "new"})
	if err == nil {
		t.Error("Append acknowledged despite quarantine being unavailable")
	}
	got, _ := os.ReadFile(store)
	if string(got) != string(partial) {
		t.Errorf("crash tail destroyed on quarantine failure: got %q want %q", got, partial)
	}
}

// When quarantine succeeds, the partial tail is moved to .corrupt (durably) and
// the store is repaired + appended as normal.
func TestAppendQuarantinesPartialTail(t *testing.T) {
	store := filepath.Join(t.TempDir(), "meta.jsonl")
	partial := []byte(`{"action_id":99`)
	if err := os.WriteFile(store, partial, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Append(store, Record{Key: "s", Field: "note", Op: "set", Value: "new"}); err != nil {
		t.Fatalf("append should succeed: %v", err)
	}
	q, err := os.ReadFile(store + ".corrupt")
	if err != nil {
		t.Fatalf("quarantine file missing: %v", err)
	}
	if string(q) != string(partial)+"\n" {
		t.Errorf("quarantine content = %q want %q", q, string(partial)+"\n")
	}
	st, _ := Fold(store)
	if st["s"].Note != "new" {
		t.Errorf("repaired append lost: %+v", st)
	}
}

// A whitespace-only note is a real value the user set; import must preserve its
// exact bytes rather than silently dropping it while reporting a clean row.
func TestImportPreservesWhitespaceOnlyNote(t *testing.T) {
	d := t.TempDir()
	dbp := filepath.Join(d, "csess.db")
	db, err := sql.Open("sqlite", dbp)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE meta(session_id TEXT, fav INT, tags TEXT, note TEXT)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO meta VALUES('2026-09-27--whitespace',0,'','   ')`); err != nil {
		t.Fatal(err)
	}
	db.Close()

	store := filepath.Join(d, "meta.jsonl")
	rep, err := ImportCsess(dbp, store)
	if err != nil {
		t.Fatal(err)
	}
	st, _ := Fold(store)
	if got := st["archive:2026-09-27--whitespace"].Note; got != "   " {
		t.Errorf("exact note lost: got %q want %q (report=%+v)", got, "   ", rep)
	}
}
