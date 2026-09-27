package core

import "testing"

func TestOpenInitSchema(t *testing.T) {
	db, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	// HasFTS5 must return a bool without erroring (either value is acceptable).
	_ = db.HasFTS5()

	if err := db.InitSchema(); err != nil {
		t.Fatalf("InitSchema: %v", err)
	}
	for _, tbl := range []string{"sessions", "events", "memory", "files"} {
		if !db.TableExists(tbl) {
			t.Errorf("table %q missing", tbl)
		}
	}
	// Full-text surface exists under one of the two names.
	if db.HasFTS5() {
		var n string
		if err := db.SQL().QueryRow(
			"SELECT name FROM sqlite_master WHERE name='fts'").Scan(&n); err != nil {
			t.Errorf("fts5 table missing though HasFTS5 true: %v", err)
		}
	} else if !db.TableExists("terms") {
		t.Errorf("fallback terms table missing")
	}
}
