// Package core holds the rebuildable index database and shared primitives.
package core

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite" // pure-Go, cgo-free sqlite driver (driver name "sqlite")
)

// DB wraps the rebuildable cache database.
type DB struct {
	sql    *sql.DB
	hasFTS bool
}

// Open opens (creating parent dirs) the cache DB at path and probes FTS5.
func Open(path string) (*DB, error) {
	if path != ":memory:" {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return nil, err
		}
	}
	sdb, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if _, err := sdb.Exec("PRAGMA journal_mode=WAL; PRAGMA synchronous=NORMAL;"); err != nil {
		sdb.Close()
		return nil, err
	}
	db := &DB{sql: sdb}
	db.hasFTS = probeFTS5(sdb)
	return db, nil
}

// probeFTS5 attempts to create an FTS5 table inside a rolled-back savepoint.
func probeFTS5(sdb *sql.DB) bool {
	if _, err := sdb.Exec("SAVEPOINT fts_probe;"); err != nil {
		return false
	}
	_, err := sdb.Exec("CREATE VIRTUAL TABLE __fts_probe USING fts5(x);")
	sdb.Exec("ROLLBACK TO fts_probe; RELEASE fts_probe;")
	return err == nil
}

// HasFTS5 reports whether the driver supports FTS5.
func (d *DB) HasFTS5() bool { return d.hasFTS }

// SQL exposes the underlying handle for packages that build on core.
func (d *DB) SQL() *sql.DB { return d.sql }

// Close closes the database.
func (d *DB) Close() error { return d.sql.Close() }

// InitSchema creates all derived tables. The full-text surface is FTS5 when
// available, else a portable tokenized `terms` table with the same query API.
func (d *DB) InitSchema() error {
	// The cache is rebuildable: when the derived layout changes, drop it and
	// let the next reindex repopulate from source.
	if d.TableExists("state") && d.GetState("schema_ver") != schemaVer {
		for _, t := range []string{"sessions", "events", "memory", "files", "state", "fts", "terms"} {
			if _, err := d.sql.Exec(`DROP TABLE IF EXISTS ` + t); err != nil {
				return fmt.Errorf("schema reset: %w", err)
			}
		}
	}
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS sessions(
			id TEXT PRIMARY KEY, agent TEXT, title TEXT, started TEXT,
			path TEXT, kind TEXT, activity INT DEFAULT 0);`,
		`CREATE TABLE IF NOT EXISTS events(
			session_id TEXT, idx INT, role TEXT, ts TEXT, text TEXT);`,
		`CREATE INDEX IF NOT EXISTS ix_events_sid ON events(session_id, idx);`,
		`CREATE TABLE IF NOT EXISTS memory(
			name TEXT PRIMARY KEY, type TEXT, category TEXT,
			description TEXT, body TEXT, path TEXT, superseded INT DEFAULT 0);`,
		`CREATE TABLE IF NOT EXISTS files(
			path TEXT PRIMARY KEY, sha TEXT, bytes INT, mtime REAL, session_id TEXT);`,
		`CREATE TABLE IF NOT EXISTS state(key TEXT PRIMARY KEY, val TEXT);`,
	}
	if d.hasFTS {
		stmts = append(stmts,
			`CREATE VIRTUAL TABLE IF NOT EXISTS fts USING fts5(
				id UNINDEXED, agent UNINDEXED, kind UNINDEXED, title, text, aux);`)
	} else {
		stmts = append(stmts,
			`CREATE TABLE IF NOT EXISTS terms(
				id TEXT, agent TEXT, kind TEXT, title TEXT, text TEXT, aux TEXT);`,
			`CREATE INDEX IF NOT EXISTS ix_terms_id ON terms(id);`)
	}
	stmts = append(stmts, `INSERT OR REPLACE INTO state(key,val) VALUES('schema_ver','`+schemaVer+`');`)
	for _, s := range stmts {
		if _, err := d.sql.Exec(s); err != nil {
			return fmt.Errorf("schema: %w", err)
		}
	}
	return nil
}

// schemaVer identifies the derived-table layout and the indexing rules that
// fill it. Bump it whenever either changes so stale caches rebuild.
const schemaVer = "4"

// GetState returns a stored state value (empty string if absent).
func (d *DB) GetState(key string) string {
	var v string
	d.sql.QueryRow(`SELECT val FROM state WHERE key=?`, key).Scan(&v)
	return v
}

// TableExists reports whether a table (or virtual table) exists.
func (d *DB) TableExists(name string) bool {
	var n string
	err := d.sql.QueryRow(
		"SELECT name FROM sqlite_master WHERE type IN ('table') AND name=?", name).Scan(&n)
	return err == nil && n == name
}
