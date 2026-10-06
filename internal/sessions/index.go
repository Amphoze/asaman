// Package sessions builds the searchable index over all adapters' sessions
// plus the memory facts, and answers unified queries.
package sessions

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/memory"
)

// Reindex incrementally refreshes the derived tables. It parses only source
// files whose content hash changed since the last run (via the `files` table),
// removes rows for deleted files, and rebuilds the memory rows only when the
// facts' signature changed. When nothing changed it does no work at all — no
// file is re-parsed.
func Reindex(db *core.DB, ads []adapters.Adapter, facts []memory.Fact) error {
	sq := db.SQL()
	ftsTable := "fts"
	if !db.HasFTS5() {
		ftsTable = "terms"
	}

	// Current source-file inventory + owning adapter.
	cur := map[string]string{} // path -> sha
	owner := map[string]adapters.Adapter{}
	for _, a := range ads {
		for _, p := range a.Files() {
			sha, err := core.FileSHA(p)
			if err != nil {
				continue
			}
			cur[p] = sha
			owner[p] = a
		}
	}

	// Previous inventory from the files table.
	old := map[string]string{}    // path -> sha
	oldSid := map[string]string{} // path -> session_id
	if rows, err := sq.Query(`SELECT path, sha, session_id FROM files`); err == nil {
		for rows.Next() {
			var p, s, sid string
			if err := rows.Scan(&p, &s, &sid); err == nil {
				old[p] = s
				oldSid[p] = sid
			}
		}
		rows.Close()
	}

	var added, edited, deleted []string
	for p, sh := range cur {
		if o, ok := old[p]; !ok {
			added = append(added, p)
		} else if o != sh {
			edited = append(edited, p)
		}
	}
	for p := range old {
		if _, ok := cur[p]; !ok {
			deleted = append(deleted, p)
		}
	}
	sessionsChanged := len(added)+len(edited)+len(deleted) > 0

	memSig := factsSignature(facts)
	memChanged := memSig != db.GetState("mem_sig")

	if !sessionsChanged && !memChanged {
		return nil // nothing to do; no re-parse
	}

	tx, err := sq.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	delSession := func(sid string) error {
		if sid == "" {
			return nil
		}
		if _, err := tx.Exec(`DELETE FROM sessions WHERE id=?`, sid); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM events WHERE session_id=?`, sid); err != nil {
			return err
		}
		_, err := tx.Exec(`DELETE FROM `+ftsTable+` WHERE id=?`, sid)
		return err
	}

	// Deletions.
	for _, p := range deleted {
		if err := delSession(oldSid[p]); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM files WHERE path=?`, p); err != nil {
			return err
		}
	}

	// Added + edited: parse only these files.
	insSess, err := tx.Prepare(`INSERT OR REPLACE INTO sessions(id,agent,title,started,path,kind,activity) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	insEvt, err := tx.Prepare(`INSERT INTO events(session_id,idx,role,ts,text) VALUES(?,?,?,?,?)`)
	if err != nil {
		return err
	}
	insFts, err := tx.Prepare(`INSERT INTO ` + ftsTable + `(id,agent,kind,title,text,aux) VALUES(?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	for _, p := range append(append([]string{}, added...), edited...) {
		// On edit, remove the previous session rows at this path first.
		if sid, ok := oldSid[p]; ok {
			if err := delSession(sid); err != nil {
				return err
			}
		}
		refs, err := owner[p].ParseFile(p)
		if err != nil {
			return err
		}
		var lastSid string
		for _, r := range refs {
			if err := delSession(r.ID); err != nil { // idempotent upsert
				return err
			}
			if _, err := insSess.Exec(r.ID, r.Agent, r.Title, r.Started, r.Path, r.Kind, r.Activity); err != nil {
				return err
			}
			// Searchable text is split by provenance: what people and agents
			// wrote (text) ranks above tool output and compaction summaries
			// (aux); injected context is not searchable at all.
			var prose, aux strings.Builder
			for _, e := range r.Events {
				if _, err := insEvt.Exec(r.ID, e.Idx, e.Role, e.Ts, e.Text); err != nil {
					return err
				}
				switch e.Role {
				case adapters.RoleContext:
				case "user", "assistant", adapters.RoleArchive:
					prose.WriteString(e.Text)
					prose.WriteByte('\n')
				default:
					aux.WriteString(e.Text)
					aux.WriteByte('\n')
				}
			}
			if _, err := insFts.Exec(r.ID, r.Agent, r.Kind, r.Title, prose.String(), aux.String()); err != nil {
				return err
			}
			lastSid = r.ID
		}
		if _, err := tx.Exec(
			`INSERT OR REPLACE INTO files(path,sha,bytes,mtime,session_id) VALUES(?,?,?,?,?)`,
			p, cur[p], 0, 0, lastSid); err != nil {
			return err
		}
	}

	// Memory rows: rebuilt wholesale only when the facts changed (cheap; small).
	if memChanged {
		if _, err := tx.Exec(`DELETE FROM memory`); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM ` + ftsTable + ` WHERE agent='memory'`); err != nil {
			return err
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
			if _, err := insFts.Exec(f.Name, "memory", "memory", f.Name+" — "+f.Description, f.Body, ""); err != nil {
				return err
			}
		}
		if _, err := tx.Exec(`INSERT OR REPLACE INTO state(key,val) VALUES('mem_sig',?)`, memSig); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// factsSignature is a content hash over all facts, order-independent.
func factsSignature(facts []memory.Fact) string {
	lines := make([]string, 0, len(facts))
	for _, f := range facts {
		lines = append(lines, f.Name+"\x1f"+f.Type+"\x1f"+f.Category+"\x1f"+f.Description+"\x1f"+f.Body)
	}
	sort.Strings(lines)
	h := sha256.New()
	for _, l := range lines {
		h.Write([]byte(l))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
