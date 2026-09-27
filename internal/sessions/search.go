package sessions

import (
	"strings"

	"github.com/amphoze/asaman/internal/core"
)

// Hit is one search result.
type Hit struct {
	ID      string
	Agent   string
	Kind    string
	Title   string
	Snippet string
}

// Opts filters a search.
type Opts struct {
	SessionsOnly bool
	MemoryOnly   bool
	Agent        string
	Limit        int
}

// Search runs a unified query across sessions and memory.
func Search(db *core.DB, q string, o Opts) ([]Hit, error) {
	if o.Limit <= 0 {
		o.Limit = 50
	}
	if db.HasFTS5() {
		return searchFTS(db, q, o)
	}
	return searchLike(db, q, o)
}

func kindFilter(o Opts) (string, []any) {
	var clauses []string
	var args []any
	if o.SessionsOnly {
		clauses = append(clauses, "kind != 'memory'")
	}
	if o.MemoryOnly {
		clauses = append(clauses, "kind = 'memory'")
	}
	if o.Agent != "" {
		clauses = append(clauses, "agent = ?")
		args = append(args, o.Agent)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

func searchFTS(db *core.DB, q string, o Opts) ([]Hit, error) {
	filter, fargs := kindFilter(o)
	// Quote the query as a single FTS5 phrase to neutralize operators.
	phrase := `"` + strings.ReplaceAll(q, `"`, `""`) + `"`
	sqlStr := `SELECT id,agent,kind,title,snippet(fts,4,'[',']','…',12) FROM fts
		WHERE fts MATCH ?` + filter + ` LIMIT ?`
	args := append([]any{phrase}, fargs...)
	args = append(args, o.Limit)
	rows, err := db.SQL().Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Agent, &h.Kind, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func searchLike(db *core.DB, q string, o Opts) ([]Hit, error) {
	filter, fargs := kindFilter(o)
	like := "%" + q + "%"
	sqlStr := `SELECT id,agent,kind,title,substr(text,1,160) FROM terms
		WHERE (title LIKE ? OR text LIKE ?)` + filter + ` LIMIT ?`
	args := append([]any{like, like}, fargs...)
	args = append(args, o.Limit)
	rows, err := db.SQL().Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		if err := rows.Scan(&h.ID, &h.Agent, &h.Kind, &h.Title, &h.Snippet); err != nil {
			return nil, err
		}
		h.Snippet = highlight(h.Snippet, q)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

func highlight(s, q string) string {
	i := strings.Index(strings.ToLower(s), strings.ToLower(q))
	if i < 0 {
		return s
	}
	return s[:i] + "[" + s[i:i+len(q)] + "]" + s[i+len(q):]
}
