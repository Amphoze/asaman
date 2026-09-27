package meta

import (
	"database/sql"
	"fmt"
	"regexp"
	"strings"

	_ "modernc.org/sqlite"
)

// Report summarizes an import run.
type Report struct {
	Mapped   int
	Unmapped int
	Details  []string
}

var importUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// mapKey converts a csess session_id into an asaman namespaced key.
// A bare UUID is a Claude transcript; anything else is an archive stem.
func mapKey(csessID string) (string, bool) {
	id := strings.TrimSpace(csessID)
	if id == "" {
		return "", false
	}
	if importUUID.MatchString(id) {
		return "claude:" + id, true
	}
	return "archive:" + id, true
}

// ImportCsess reads the csess meta table and appends equivalent records to the
// asaman store. Idempotent-friendly at the fold level (fav/note replacement,
// tags are ordered-set adds). Any unmapped/empty row is reported (blocks
// deletion upstream).
func ImportCsess(csessDB, store string) (Report, error) {
	var rep Report
	db, err := sql.Open("sqlite", "file:"+csessDB+"?mode=ro")
	if err != nil {
		return rep, err
	}
	defer db.Close()
	rows, err := db.Query(`SELECT session_id, fav, tags, note FROM meta`)
	if err != nil {
		return rep, err
	}
	defer rows.Close()
	for rows.Next() {
		var sid, tags, note string
		var fav int
		if err := rows.Scan(&sid, &fav, &tags, &note); err != nil {
			return rep, err
		}
		key, ok := mapKey(sid)
		if !ok {
			rep.Unmapped++
			rep.Details = append(rep.Details, fmt.Sprintf("unmapped session_id=%q", sid))
			continue
		}
		if fav != 0 {
			if _, err := Append(store, Record{Actor: "import", Key: key, Field: "fav", Op: "set", Value: "1"}); err != nil {
				return rep, err
			}
		}
		for _, t := range splitTags(tags) {
			if _, err := Append(store, Record{Actor: "import", Key: key, Field: "tag", Op: "add", Value: t}); err != nil {
				return rep, err
			}
		}
		if strings.TrimSpace(note) != "" {
			if _, err := Append(store, Record{Actor: "import", Key: key, Field: "note", Op: "set", Value: note}); err != nil {
				return rep, err
			}
		}
		rep.Mapped++
	}
	return rep, rows.Err()
}

func splitTags(s string) []string {
	var out []string
	for _, t := range strings.Split(s, ",") {
		if t = strings.TrimSpace(t); t != "" {
			out = append(out, t)
		}
	}
	return out
}
