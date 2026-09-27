// Package sessions builds the searchable index over all adapters' sessions
// plus the memory facts, and answers unified queries.
package sessions

import (
	"strings"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/memory"
)

// Reindex rebuilds all derived tables from the adapters' sessions and the facts.
func Reindex(db *core.DB, ads []adapters.Adapter, facts []memory.Fact) error {
	sq := db.SQL()
	tx, err := sq.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	for _, t := range []string{"sessions", "events", "memory", "files"} {
		if _, err := tx.Exec("DELETE FROM " + t); err != nil {
			return err
		}
	}
	ftsTable := "fts"
	if !db.HasFTS5() {
		ftsTable = "terms"
	}
	if _, err := tx.Exec("DELETE FROM " + ftsTable); err != nil {
		return err
	}

	insSess, err := tx.Prepare(`INSERT OR REPLACE INTO sessions(id,agent,title,started,path,kind,activity) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	insEvt, err := tx.Prepare(`INSERT INTO events(session_id,idx,role,ts,text) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	insFts, err := tx.Prepare(`INSERT INTO ` + ftsTable + `(id,agent,kind,title,text) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}

	for _, a := range ads {
		refs, err := a.Sessions()
		if err != nil {
			return err
		}
		for _, r := range refs {
			if _, err := insSess.Exec(r.ID, r.Agent, r.Title, r.Started, r.Path, r.Kind, r.Activity); err != nil {
				return err
			}
			var sb strings.Builder
			for _, e := range r.Events {
				if _, err := insEvt.Exec(r.ID, e.Idx, e.Role, e.Ts, e.Text); err != nil {
					return err
				}
				sb.WriteString(e.Text)
				sb.WriteByte('\n')
			}
			if _, err := insFts.Exec(r.ID, r.Agent, r.Kind, r.Title, sb.String()); err != nil {
				return err
			}
		}
	}

	insMem, err := tx.Prepare(`INSERT OR REPLACE INTO memory(name,type,category,description,body,path,superseded) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, f := range facts {
		sup := 0
		if f.Superseded() {
			sup = 1
		}
		if _, err := insMem.Exec(f.Name, f.Type, f.Category, f.Description, f.Body, f.Path, sup); err != nil {
			return err
		}
		if _, err := insFts.Exec(f.Name, "memory", "memory", f.Description, f.Body); err != nil {
			return err
		}
	}
	return tx.Commit()
}
