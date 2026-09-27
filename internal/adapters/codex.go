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

func init() { Register("codex", newCodex) }

type codexAdapter struct {
	name    string
	cfg     config.AgentCfg
	markers [2]string
	lock    string
}

func newCodex(name string, cfg config.AgentCfg, markers [2]string, lock string) Adapter {
	return &codexAdapter{name: name, cfg: cfg, markers: markers, lock: lock}
}

func (a *codexAdapter) Name() string           { return a.name }
func (a *codexAdapter) TimeLog() []string      { return a.cfg.TimeLog }
func (a *codexAdapter) ContextPath() string    { return a.cfg.ContextFile }
func (a *codexAdapter) MemoryMode() MemoryMode { return MemoryMode(a.cfg.MemoryMode) }
func (a *codexAdapter) SyncIndex(index string) error {
	return contextpkg.SyncIndex(a.cfg.ContextFile, index, a.markers, a.lock)
}

var uuidRe = regexp.MustCompile(`[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}`)

// Files lists Codex rollout transcript files.
func (a *codexAdapter) Files() []string {
	var out []string
	for _, g := range a.cfg.Sessions {
		out = append(out, core.ExpandGlob(g)...)
	}
	return out
}

// ParseFile parses one Codex rollout transcript into a SessionRef.
func (a *codexAdapter) ParseFile(p string) ([]SessionRef, error) {
	titles := a.loadTitles()
	sr := parseCodexTranscript(p)
	if sr.ID == "" {
		sr.ID = uuidRe.FindString(filepath.Base(p))
	}
	uuid := sr.ID
	sr.ID = "codex:" + uuid
	sr.Agent = "codex"
	sr.Kind = "session"
	if t, ok := titles[uuid]; ok && t != "" {
		sr.Title = t
	}
	return []SessionRef{sr}, nil
}

func (a *codexAdapter) Sessions() ([]SessionRef, error) {
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

// loadTitles best-effort reads session_index.jsonl for uuid→title.
func (a *codexAdapter) loadTitles() map[string]string {
	titles := map[string]string{}
	for _, g := range a.cfg.Meta {
		for _, p := range core.ExpandGlob(g) {
			if !strings.Contains(filepath.Base(p), "session_index") {
				continue
			}
			f, err := os.Open(p)
			if err != nil {
				continue
			}
			sc := bufio.NewScanner(f)
			sc.Buffer(make([]byte, 1024*1024), 8*1024*1024)
			for sc.Scan() {
				var m map[string]any
				if json.Unmarshal(sc.Bytes(), &m) != nil {
					continue
				}
				id := uuidRe.FindString(str(m["id"]) + " " + str(m["session_id"]) + " " + str(m["path"]))
				title := firstNonEmpty(str(m["title"]), str(m["preview"]), str(m["summary"]))
				if id != "" && title != "" {
					titles[id] = title
				}
			}
			f.Close()
		}
	}
	return titles
}

type codexLine struct {
	Timestamp string          `json:"timestamp"`
	Type      string          `json:"type"`
	Payload   json.RawMessage `json:"payload"`
}

func parseCodexTranscript(path string) SessionRef {
	sr := SessionRef{Path: path}
	f, err := os.Open(path)
	if err != nil {
		return sr
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1024*1024), 32*1024*1024)
	idx := 0
	for sc.Scan() {
		var l codexLine
		if json.Unmarshal(sc.Bytes(), &l) != nil {
			continue // skip malformed
		}
		if sr.Started == "" && l.Timestamp != "" {
			sr.Started = l.Timestamp
		}
		switch l.Type {
		case "session_meta":
			var p struct {
				SessionID string `json:"session_id"`
				ID        string `json:"id"`
			}
			if json.Unmarshal(l.Payload, &p) == nil {
				if id := firstNonEmpty(p.SessionID, p.ID); id != "" {
					sr.ID = uuidRe.FindString(id)
				}
			}
		case "response_item":
			role, text := codexItem(l.Payload)
			if text == "" {
				continue
			}
			sr.Events = append(sr.Events, Event{Idx: idx, Role: role, Ts: l.Timestamp, Text: text})
			idx++
			if sr.Title == "" && role == "user" {
				sr.Title = firstLine(text)
			}
		}
	}
	sr.Activity = len(sr.Events)
	if sr.Title == "" {
		sr.Title = strings.TrimSuffix(filepath.Base(path), ".jsonl")
	}
	return sr
}

// codexItem extracts (role, text) from a response_item payload: messages,
// function calls (tools), and their outputs.
func codexItem(raw json.RawMessage) (string, string) {
	var p struct {
		Type    string          `json:"type"`
		Role    string          `json:"role"`
		Name    string          `json:"name"`
		Content json.RawMessage `json:"content"`
		Output  json.RawMessage `json:"output"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return "", ""
	}
	switch p.Type {
	case "message":
		return firstNonEmpty(p.Role, "assistant"), codexContent(p.Content)
	case "function_call", "local_shell_call", "custom_tool_call":
		return "tool", "[tool:" + firstNonEmpty(p.Name, p.Type) + "]"
	case "function_call_output", "custom_tool_call_output":
		return "tool_result", codexContent(p.Output)
	case "reasoning":
		return "assistant", codexContent(p.Content)
	}
	return "", ""
}

// codexContent flattens a content value that is a string or an array of blocks
// each carrying a `text` field (input_text / output_text / text).
func codexContent(raw json.RawMessage) string {
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
		if t, ok := b["text"].(string); ok && t != "" {
			parts = append(parts, t)
		}
	}
	return strings.TrimSpace(strings.Join(parts, "\n"))
}

func str(v any) string {
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}
func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if v != "" {
			return v
		}
	}
	return ""
}
