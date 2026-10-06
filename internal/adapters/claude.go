package adapters

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/amphoze/asaman/internal/config"
	contextpkg "github.com/amphoze/asaman/internal/context"
	"github.com/amphoze/asaman/internal/core"
)

func init() { Register("claude", newClaude) }

type claudeAdapter struct {
	name    string
	cfg     config.AgentCfg
	markers [2]string
	lock    string
}

func newClaude(name string, cfg config.AgentCfg, markers [2]string, lock string) Adapter {
	return &claudeAdapter{name: name, cfg: cfg, markers: markers, lock: lock}
}

func (a *claudeAdapter) Name() string           { return a.name }
func (a *claudeAdapter) TimeLog() []string      { return a.cfg.TimeLog }
func (a *claudeAdapter) ContextPath() string    { return a.cfg.ContextFile }
func (a *claudeAdapter) MemoryMode() MemoryMode { return MemoryMode(a.cfg.MemoryMode) }

func (a *claudeAdapter) SyncIndex(index string) error {
	return contextpkg.SyncIndex(a.cfg.ContextFile, index, a.markers, a.lock)
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

// Files lists transcript files (skipping the sessions.jsonl time-log) plus
// archive markdown.
func (a *claudeAdapter) Files() []string {
	var out []string
	for _, g := range a.cfg.Sessions {
		for _, p := range core.ExpandGlob(g) {
			if filepath.Base(p) == "sessions.jsonl" {
				continue
			}
			out = append(out, p)
		}
	}
	for _, g := range a.cfg.Archive {
		out = append(out, core.ExpandGlob(g)...)
	}
	return out
}

// ParseFile parses one transcript (.jsonl) or archive (.md) file.
func (a *claudeAdapter) ParseFile(p string) ([]SessionRef, error) {
	if strings.HasSuffix(p, ".md") {
		root := filepath.Dir(p)
		for _, g := range a.cfg.Archive {
			if r := archiveRoot(g); strings.HasPrefix(p, r) {
				root = r
				break
			}
		}
		id, kind := archiveID(root, p)
		sr := SessionRef{
			ID: "archive:" + id, Agent: "claude", Kind: kind, Path: p,
			Title:   strings.TrimSuffix(filepath.Base(p), ".md"),
			Started: dateRe.FindString(filepath.Base(p)),
		}
		// Index the note body: the archive is the only record of old sessions.
		if b, err := os.ReadFile(p); err == nil {
			if body := strings.TrimSpace(string(b)); body != "" {
				sr.Events = []Event{{Role: RoleArchive, Ts: sr.Started, Text: body}}
				sr.Activity = 1
			}
		}
		return []SessionRef{sr}, nil
	}
	if filepath.Base(p) == "sessions.jsonl" {
		return nil, nil
	}
	kind := "session"
	if strings.Contains(p, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
		kind = "subagent"
	}
	sr := parseClaudeTranscript(p)
	sr.ID = "claude:" + strings.TrimSuffix(filepath.Base(p), ".jsonl")
	sr.Agent = "claude"
	sr.Kind = kind
	return []SessionRef{sr}, nil
}

func (a *claudeAdapter) Sessions() ([]SessionRef, error) {
	var out []SessionRef
	for _, p := range a.Files() {
		refs, err := a.ParseFile(p)
		if err != nil {
			return nil, err
		}
		out = append(out, refs...)
	}
	return out, nil
}

// archiveRoot returns the fixed directory prefix before a `**` in the glob.
func archiveRoot(glob string) string {
	if i := strings.Index(glob, "**"); i >= 0 {
		return strings.TrimRight(glob[:i], string(filepath.Separator))
	}
	return filepath.Dir(glob)
}

// archiveID mirrors csess scan_archive: dated `YYYY-MM-DD--slug.md` at depth 3
// → stem; anything else → joined path segments.
func archiveID(root, path string) (id, kind string) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		rel = path
	}
	parts := strings.Split(rel, string(filepath.Separator))
	stem := strings.TrimSuffix(filepath.Base(path), ".md")
	if len(parts) == 3 && dateRe.MatchString(stem) {
		return stem, "session"
	}
	if len(parts) > 1 {
		joined := strings.Join(append(parts[:len(parts)-1], stem), "--")
		if joined != "" {
			return joined, "report"
		}
	}
	return stem, "report"
}

// claudeLine is the subset of a transcript record we read.
type claudeLine struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	Summary   string          `json:"summary"`
	Message   json.RawMessage `json:"message"`
	IsMeta    bool            `json:"isMeta"`
	IsCompact bool            `json:"isCompactSummary"`
}

type claudeMsg struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

func parseClaudeTranscript(path string) SessionRef {
	sr := SessionRef{Path: path}
	f, err := os.Open(path)
	if err != nil {
		return sr
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 16*1024*1024)
	idx := 0
	for sc.Scan() {
		var l claudeLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue // skip malformed line
		}
		if sr.Started == "" && l.Timestamp != "" {
			sr.Started = l.Timestamp
		}
		if l.Type == "summary" && l.Summary != "" && sr.Title == "" {
			sr.Title = l.Summary
		}
		role, text := "", ""
		if len(l.Message) > 0 {
			var m claudeMsg
			if json.Unmarshal(l.Message, &m) == nil {
				var toolOnly bool
				text, toolOnly = extractText(m.Content)
				role = m.Role
				if toolOnly {
					role = "tool_result" // tool output arrives under the user role
				}
			}
		}
		if role == "" {
			role = l.Type
		}
		if text == "" && l.Summary != "" {
			text = l.Summary
		}
		switch {
		case l.IsMeta || IsContext(text):
			role = RoleContext
		case l.IsCompact:
			role = RoleSummary
		default:
			text = StripInjected(text)
		}
		if text == "" {
			continue
		}
		sr.Events = append(sr.Events, Event{Idx: idx, Role: role, Ts: l.Timestamp, Text: text})
		idx++
		if sr.Title == "" && role == "user" {
			sr.Title = titleLine(text)
		}
	}
	sr.Activity = len(sr.Events)
	if sr.Title == "" {
		sr.Title = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return sr
}

// extractText pulls plain text + tool names/results from Claude content, which
// is either a string or an array of typed blocks. toolOnly is true when the
// content carried tool results and no prose.
func extractText(raw json.RawMessage) (text string, toolOnly bool) {
	if len(raw) == 0 {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, false
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return "", false
	}
	var parts []string
	prose, results := 0, 0
	for _, b := range blocks {
		switch b["type"] {
		case "text":
			if t, ok := b["text"].(string); ok {
				parts = append(parts, t)
				prose++
			}
		case "tool_use":
			if n, ok := b["name"].(string); ok {
				parts = append(parts, "[tool:"+n+"]")
			}
		case "tool_result":
			results++
			switch c := b["content"].(type) {
			case string:
				parts = append(parts, c)
			case []any:
				for _, it := range c {
					if m, ok := it.(map[string]any); ok {
						if t, ok := m["text"].(string); ok {
							parts = append(parts, t)
						}
					}
				}
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n")), results > 0 && prose == 0
}
