package sessions

import (
	"strings"
	"unicode"

	"github.com/amphoze/asaman/internal/core"
)

// Hit is one search result.
type Hit struct {
	ID      string
	Agent   string
	Kind    string
	Title   string
	Snippet string
	Started string // session start (empty for memory facts)
	Path    string // source file
	Similar int    // further hits collapsed into this one (same matched text)
	Loose   bool   // terms all present but far apart: weak evidence
}

// Opts filters a search.
type Opts struct {
	SessionsOnly bool
	MemoryOnly   bool
	Agent        string
	Limit        int
}

// Search runs a unified query across sessions and memory. Results are ranked:
// sessions containing the exact phrase first, then sessions containing every
// term close together; within each, matches in titles and prose outrank
// matches in tool output, and newer sessions break ties.
func Search(db *core.DB, q string, o Opts) ([]Hit, error) {
	if o.Limit <= 0 {
		o.Limit = 20
	}
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	var hits []Hit
	var err error
	if db.HasFTS5() {
		hits, err = searchFTS(db, q, o)
	} else {
		hits, err = searchLike(db, q, o)
	}
	if err != nil {
		return nil, err
	}
	hits = collapse(hits)
	if len(hits) > o.Limit {
		hits = hits[:o.Limit]
	}
	return hits, nil
}

func kindFilter(o Opts) (string, []any) {
	var clauses []string
	var args []any
	if o.SessionsOnly {
		clauses = append(clauses, "f.kind != 'memory'")
	}
	if o.MemoryOnly {
		clauses = append(clauses, "f.kind = 'memory'")
	}
	if o.Agent != "" {
		clauses = append(clauses, "f.agent = ?")
		args = append(args, o.Agent)
	}
	if len(clauses) == 0 {
		return "", args
	}
	return " AND " + strings.Join(clauses, " AND "), args
}

// terms splits a query into word tokens the way the FTS tokenizer would.
func terms(q string) []string {
	return strings.FieldsFunc(q, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
}

func quote(s string) string { return `"` + strings.ReplaceAll(s, `"`, `""`) + `"` }

// overfetch leaves room for collapse() to fold duplicates before the limit.
const overfetch = 5

func searchFTS(db *core.DB, q string, o Opts) ([]Hit, error) {
	filter, fargs := kindFilter(o)
	filter = strings.ReplaceAll(filter, "f.", "fts.")
	// Column weights: title, prose, tool output/summaries.
	sqlStr := `SELECT fts.id, fts.agent, fts.kind, fts.title,
			snippet(fts,3,char(1),char(2),'…',12), snippet(fts,4,char(1),char(2),'…',12), snippet(fts,5,char(1),char(2),'…',12),
			COALESCE(s.started,''), COALESCE(s.path,'')
		FROM fts LEFT JOIN sessions s ON s.id = fts.id
		WHERE fts MATCH ?` + filter + `
		ORDER BY bm25(fts, 0, 0, 0, 10.0, 4.0, 0.5), COALESCE(s.started,'') DESC
		LIMIT ?`
	run := func(match string) ([]Hit, error) {
		args := append([]any{match}, fargs...)
		args = append(args, o.Limit*overfetch)
		rows, err := db.SQL().Query(sqlStr, args...)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		var hits []Hit
		for rows.Next() {
			var h Hit
			var st, sp, sa string
			if err := rows.Scan(&h.ID, &h.Agent, &h.Kind, &h.Title, &st, &sp, &sa, &h.Started, &h.Path); err != nil {
				return nil, err
			}
			// Show the strongest evidence: prose, then title, then tool output.
			h.Snippet = sa
			for _, c := range []string{sp, st} {
				if strings.Contains(c, "\x01") {
					h.Snippet = c
					break
				}
			}
			h.Snippet = markRe.Replace(h.Snippet)
			hits = append(hits, h)
		}
		return hits, rows.Err()
	}

	// Pass 1: the exact phrase. Pass 2: every term close together. Pass 3,
	// only when those found little: every term anywhere in the session. In a
	// long transcript that is weak evidence, so it is capped and flagged.
	hits, err := run(quote(q))
	if err != nil {
		return nil, err
	}
	ts := terms(q)
	if len(ts) < 2 {
		return hits, nil
	}
	for i, t := range ts {
		ts[i] = quote(t)
	}
	seen := map[string]bool{}
	for _, h := range hits {
		seen[h.ID] = true
	}
	near, err := run("NEAR(" + strings.Join(ts, " ") + ", " + nearSpan + ")")
	if err != nil {
		return nil, err
	}
	for _, h := range near {
		if !seen[h.ID] {
			seen[h.ID] = true
			hits = append(hits, h)
		}
	}
	if len(hits) >= looseBelow {
		return hits, nil
	}
	loose, err := run(strings.Join(ts, " AND "))
	if err != nil {
		return nil, err
	}
	n := 0
	for _, h := range loose {
		if seen[h.ID] {
			continue
		}
		if n++; n > looseMax {
			break
		}
		h.Loose = true
		hits = append(hits, h)
	}
	return hits, nil
}

// looseBelow is the count of strong hits under which loose (terms-anywhere)
// matches are added; looseMax caps how many.
const (
	looseBelow = 5
	looseMax   = 5
)

// markRe turns the internal match markers into the displayed brackets.
var markRe = strings.NewReplacer("\x01", "[", "\x02", "]")

// nearSpan is how many tokens may separate the terms of a multi-word query.
const nearSpan = "40"

func searchLike(db *core.DB, q string, o Opts) ([]Hit, error) {
	filter, fargs := kindFilter(o)
	like := "%" + q + "%"
	sqlStr := `SELECT f.id, f.agent, f.kind, f.title,
			CASE WHEN f.text LIKE ? THEN f.text WHEN f.title LIKE ? THEN f.title ELSE f.aux END,
			COALESCE(s.started,''), COALESCE(s.path,'')
		FROM terms f LEFT JOIN sessions s ON s.id = f.id
		WHERE (f.title LIKE ? OR f.text LIKE ? OR f.aux LIKE ?)` + filter + `
		ORDER BY (f.title LIKE ?) DESC, (f.text LIKE ?) DESC, COALESCE(s.started,'') DESC
		LIMIT ?`
	args := append([]any{like, like, like, like, like}, fargs...)
	args = append(args, like, like, o.Limit*overfetch)
	rows, err := db.SQL().Query(sqlStr, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var hits []Hit
	for rows.Next() {
		var h Hit
		var body string
		if err := rows.Scan(&h.ID, &h.Agent, &h.Kind, &h.Title, &body, &h.Started, &h.Path); err != nil {
			return nil, err
		}
		h.Snippet = window(body, q)
		hits = append(hits, h)
	}
	return hits, rows.Err()
}

// window returns the text around the first occurrence of q, with q bracketed.
func window(s, q string) string {
	i := strings.Index(strings.ToLower(s), strings.ToLower(q))
	if i < 0 || i+len(q) > len(s) {
		if len(s) > 160 {
			return s[:160]
		}
		return s
	}
	lo, hi := i-60, i+len(q)+60
	if lo < 0 {
		lo = 0
	}
	if hi > len(s) {
		hi = len(s)
	}
	return strings.ToValidUTF8(s[lo:i], "") + "[" + s[i:i+len(q)] + "]" + strings.ToValidUTF8(s[i+len(q):hi], "")
}

// collapse folds hits whose matched text is identical into the first (best
// ranked) one. The same passage surfacing in many sessions is a shared
// document being re-read, not many independent discussions.
func collapse(hits []Hit) []Hit {
	first := map[string]int{}
	var out []Hit
	for _, h := range hits {
		key := strings.Join(strings.Fields(strings.ToLower(h.Snippet)), " ")
		if i, ok := first[key]; ok && key != "" {
			out[i].Similar++
			continue
		}
		first[key] = len(out)
		out = append(out, h)
	}
	return out
}
