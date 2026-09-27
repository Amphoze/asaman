package meta

import (
	"database/sql"
	"path/filepath"
	"reflect"
	"testing"

	_ "modernc.org/sqlite"
)

func makeCsessFixture(t *testing.T) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "csess.db")
	db, err := sql.Open("sqlite", p)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE meta(session_id TEXT PRIMARY KEY, fav INT DEFAULT 0, tags TEXT DEFAULT '', note TEXT DEFAULT '', updated TEXT)`); err != nil {
		t.Fatal(err)
	}
	db.Exec(`INSERT INTO meta VALUES(?,?,?,?,?)`,
		"11111111-1111-1111-1111-111111111111", 1, "alpha,beta", "keep this", "t1")
	db.Exec(`INSERT INTO meta VALUES(?,?,?,?,?)`,
		"2026-09-27--reorg", 0, "gamma", "", "t2")
	return p
}

// GATE: exact import equality across both id namespaces; zero unmapped.
func TestImportCsessExactEquality(t *testing.T) {
	csess := makeCsessFixture(t)
	store := filepath.Join(t.TempDir(), "meta.jsonl")

	rep, err := ImportCsess(csess, store)
	if err != nil {
		t.Fatal(err)
	}
	if rep.Mapped != 2 || rep.Unmapped != 0 {
		t.Fatalf("report mapped=%d unmapped=%d details=%v", rep.Mapped, rep.Unmapped, rep.Details)
	}

	st, err := Fold(store)
	if err != nil {
		t.Fatal(err)
	}
	// UUID row → claude: namespace, exact values
	c1 := st["claude:11111111-1111-1111-1111-111111111111"]
	if !c1.Fav || !reflect.DeepEqual(c1.Tags, []string{"alpha", "beta"}) || c1.Note != "keep this" {
		t.Errorf("claude row not exact: %+v", c1)
	}
	// archive stem row → archive: namespace, exact values
	c2 := st["archive:2026-09-27--reorg"]
	if c2.Fav || !reflect.DeepEqual(c2.Tags, []string{"gamma"}) || c2.Note != "" {
		t.Errorf("archive row not exact: %+v", c2)
	}

	// idempotent-at-fold: re-running import keeps equal folded state
	if _, err := ImportCsess(csess, store); err != nil {
		t.Fatal(err)
	}
	st2, _ := Fold(store)
	if !reflect.DeepEqual(st2["claude:11111111-1111-1111-1111-111111111111"], c1) {
		t.Errorf("re-import changed folded state: %+v", st2["claude:11111111-1111-1111-1111-111111111111"])
	}
}
