package adapters

import (
	"path/filepath"
	"testing"

	"github.com/amphoze/asaman/internal/config"
)

func codexCfg(t *testing.T) config.AgentCfg {
	t.Helper()
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "codex"))
	cbase, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "Codex"))
	return config.AgentCfg{
		Adapter:     "codex",
		Sessions:    []string{filepath.Join(base, "sessions", "**", "*.jsonl")},
		Meta:        []string{filepath.Join(base, "meta", "session_index.jsonl")},
		TimeLog:     []string{filepath.Join(cbase, "projects", "*", "sessions.jsonl")},
		ContextFile: filepath.Join(base, "AGENTS.md"),
		MemoryMode:  "inline-index",
	}
}

// GATE: Codex ingestion — assistant + tool events searchable, stable id,
// title from session_index, capital .Codex time-log wired.
func TestCodexIngestion(t *testing.T) {
	cfg := codexCfg(t)
	a := newCodex("codex", cfg, [2]string{"<!--a-->", "<!--/a-->"}, filepath.Join(t.TempDir(), ".lock"))
	refs, err := a.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	if len(refs) != 1 {
		t.Fatalf("want 1 codex session, got %d", len(refs))
	}
	s := refs[0]
	if s.ID != "codex:33333333-3333-4333-8333-333333333333" {
		t.Errorf("stable id wrong: %q", s.ID)
	}
	if s.Title != "Mail admin deploy" {
		t.Errorf("title from session_index not applied: %q", s.Title)
	}
	if s.Started == "" {
		t.Error("started not set from session_meta")
	}
	// events: user, assistant, tool, tool_result
	roles := map[string]bool{}
	var texts string
	for _, e := range s.Events {
		roles[e.Role] = true
		texts += e.Text + "|"
	}
	for _, want := range []string{"user", "assistant", "tool", "tool_result"} {
		if !roles[want] {
			t.Errorf("missing role %q in events (%v)", want, roles)
		}
	}
	if !contains(texts, "deploy the mail admin portal") || !contains(texts, "[tool:shell]") || !contains(texts, "deploy succeeded") {
		t.Errorf("assistant/tool events not searchable: %q", texts)
	}
	// stable id across re-scan
	refs2, _ := a.Sessions()
	if refs2[0].ID != s.ID {
		t.Error("id not stable across re-scan")
	}
	// capital .Codex time-log wired verbatim
	if len(a.TimeLog()) != 1 || !contains(a.TimeLog()[0], "/Codex/") {
		t.Errorf("capital .Codex time-log lost: %v", a.TimeLog())
	}
}
