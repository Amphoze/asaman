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

func (a *claudeAdapter) Name() string          { return a.name }
func (a *claudeAdapter) TimeLog() []string     { return a.cfg.TimeLog }
func (a *claudeAdapter) ContextPath() string   { return a.cfg.ContextFile }
func (a *claudeAdapter) MemoryMode() MemoryMode { return MemoryMode(a.cfg.MemoryMode) }

func (a *claudeAdapter) SyncIndex(index string) error {
	return contextpkg.SyncIndex(a.cfg.ContextFile, index, a.markers, a.lock)
}

var dateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}`)

func (a *claudeAdapter) Sessions() ([]SessionRef, error) {
	var out []SessionRef
	// Transcripts (skip the sessions.jsonl time-log).
	for _, g := range a.cfg.Sessions {
		for _, p := range core.ExpandGlob(g) {
			if filepath.Base(p) == "sessions.jsonl" {
				continue
			}
			kind := "session"
			if strings.Contains(p, string(filepath.Separator)+"subagents"+string(filepath.Separator)) {
				kind = "subagent"
			}
			sr := parseClaudeTranscript(p)
			sr.ID = "claude:" + strings.TrimSuffix(filepath.Base(p), ".jsonl")
			sr.Agent = "claude"
			sr.Kind = kind
			out = append(out, sr)
		}
	}
	// Archive markdown → archive:{stem} (matching csess scan_archive).
	for _, g := range a.cfg.Archive {
		root := archiveRoot(g)
		for _, p := range core.ExpandGlob(g) {
			id, kind := archiveID(root, p)
			out = append(out, SessionRef{
				ID: "archive:" + id, Agent: "claude", Kind: kind, Path: p,
				Title: strings.TrimSuffix(filepath.Base(p), ".md"),
			})
		}
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
				role = m.Role
				text = extractText(m.Content)
			}
		}
		if role == "" {
			role = l.Type
		}
		if text == "" && l.Summary != "" {
			text = l.Summary
		}
		if text == "" {
			continue
		}
		sr.Events = append(sr.Events, Event{Idx: idx, Role: role, Ts: l.Timestamp, Text: text})
		idx++
		if sr.Title == "" && role == "user" {
			sr.Title = firstLine(text)
		}
	}
	sr.Activity = len(sr.Events)
	if sr.Title == "" {
		sr.Title = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return sr
}

// extractText pulls plain text + tool names/results from Claude content, which
// is either a string or an array of typed blocks.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var blocks []map[string]any
	if json.Unmarshal(raw, &blocks) != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b["type"] {
		case "text":
			if t, ok := b["text"].(string); ok {
				parts = append(parts, t)
			}
		case "tool_use":
			if n, ok := b["name"].(string); ok {
				parts = append(parts, "[tool:"+n+"]")
			}
		case "tool_result":
			if c, ok := b["content"].(string); ok {
				parts = append(parts, c)
			}
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
