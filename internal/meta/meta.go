// Package meta is the durable, append-only, crash-safe curation store
// (favourites/tags/notes) shared by CLI, UI, and import under one lock.
package meta

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/amphoze/asaman/internal/core"
)

// Record is one curation action.
type Record struct {
	ActionID int64  `json:"action_id"`
	Ts       string `json:"ts"`
	Actor    string `json:"actor"` // cli | ui | import
	Key      string `json:"key"`   // SessionID or fact name
	Field    string `json:"field"` // fav | tag | note
	Op       string `json:"op"`    // set | clear | add | remove
	Value    string `json:"value"`
}

// Curation is the folded state for one key.
type Curation struct {
	Fav  bool
	Tags []string
	Note string
}

func lockPath(store string) string { return store + ".lock" }

// quarantineTail durably appends a partial (crash) tail to the .corrupt sidecar.
// It returns an error if the bytes cannot be written and fsync'd, so the caller
// can refuse to truncate the original store.
func quarantineTail(path string, partial []byte) error {
	cf, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	buf := append(append([]byte{}, partial...), '\n')
	for len(buf) > 0 {
		n, werr := cf.Write(buf)
		if werr != nil {
			cf.Close()
			return werr
		}
		buf = buf[n:]
	}
	if serr := cf.Sync(); serr != nil { // durable before we destroy the source
		cf.Close()
		return serr
	}
	return cf.Close()
}

// readValid returns the valid whole-line records and the byte offset of the
// end of the last complete line. Any bytes after that offset are an incomplete
// (crash) tail.
func readValid(data []byte) (recs []Record, goodEnd int) {
	if len(data) == 0 {
		return nil, 0
	}
	idx := bytes.LastIndexByte(data, '\n')
	goodEnd = idx + 1 // 0 if no newline at all
	for _, line := range bytes.Split(data[:goodEnd], []byte("\n")) {
		if len(bytes.TrimSpace(line)) == 0 {
			continue
		}
		var r Record
		if json.Unmarshal(line, &r) == nil {
			recs = append(recs, r)
		}
	}
	return recs, goodEnd
}

// Append repairs any incomplete tail (quarantining it), allocates a monotonic
// ActionID from the repaired state, and durably appends r. Serialized by flock.
func Append(store string, r Record) (int64, error) {
	var assigned int64
	err := core.WithLock(lockPath(store), func() error {
		if err := os.MkdirAll(filepath.Dir(store), 0o755); err != nil {
			return err
		}
		data, err := os.ReadFile(store)
		if err != nil && !os.IsNotExist(err) {
			return err
		}
		recs, goodEnd := readValid(data)
		// Repair-before-append: durably quarantine the partial tail BEFORE
		// truncating it. If quarantine fails, do not truncate — losing the
		// crash tail while acknowledging the append would be silent data loss.
		if goodEnd < len(data) {
			if err := quarantineTail(store+".corrupt", data[goodEnd:]); err != nil {
				return err
			}
			if e := os.Truncate(store, int64(goodEnd)); e != nil {
				return e
			}
		}
		var maxID int64
		for _, x := range recs {
			if x.ActionID > maxID {
				maxID = x.ActionID
			}
		}
		r.ActionID = maxID + 1
		assigned = r.ActionID
		if r.Ts == "" {
			r.Ts = time.Now().UTC().Format(time.RFC3339Nano)
		}
		line, err := json.Marshal(r)
		if err != nil {
			return err
		}
		line = append(line, '\n')
		f, err := os.OpenFile(store, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err != nil {
			return err
		}
		// short-write loop
		for len(line) > 0 {
			n, werr := f.Write(line)
			if werr != nil {
				f.Close()
				return werr
			}
			line = line[n:]
		}
		if err := f.Sync(); err != nil { // durable acknowledgement
			f.Close()
			return err
		}
		return f.Close()
	})
	return assigned, err
}

// Fold replays the log into per-key curation state.
func Fold(store string) (map[string]Curation, error) {
	data, err := os.ReadFile(store)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]Curation{}, nil
		}
		return nil, err
	}
	recs, _ := readValid(data)
	sort.SliceStable(recs, func(i, j int) bool { return recs[i].ActionID < recs[j].ActionID })
	out := map[string]Curation{}
	for _, r := range recs {
		c := out[r.Key]
		switch r.Field {
		case "fav":
			c.Fav = r.Value == "1" || r.Value == "true"
		case "note":
			if r.Op == "clear" {
				c.Note = ""
			} else {
				c.Note = r.Value
			}
		case "tag":
			c.Tags = applyTag(c.Tags, r.Op, r.Value)
		}
		out[r.Key] = c
	}
	return out, nil
}

// applyTag maintains an ordered set of tags.
func applyTag(tags []string, op, v string) []string {
	switch op {
	case "add":
		for _, t := range tags {
			if t == v {
				return tags
			}
		}
		return append(tags, v)
	case "remove":
		out := tags[:0:0]
		for _, t := range tags {
			if t != v {
				out = append(out, t)
			}
		}
		return out
	}
	return tags
}
