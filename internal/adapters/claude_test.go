package adapters

import (
	"path/filepath"
	"testing"

	"github.com/amphoze/asaman/internal/config"
)

func claudeCfg(t *testing.T) config.AgentCfg {
	t.Helper()
	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata", "claude"))
	return config.AgentCfg{
		Adapter: "claude",
		Sessions: []string{
			filepath.Join(base, "projects", "*", "*.jsonl"),
			filepath.Join(base, "projects", "*", "subagents", "*.jsonl"),
		},
		Archive:     []string{filepath.Join(base, "archive", "**", "*.md")},
		TimeLog:     []string{filepath.Join(base, "projects", "*", "sessions.jsonl")},
		ContextFile: filepath.Join(base, "CLAUDE.md"),
		MemoryMode:  "native",
	}
}

func TestClaudeAdapter(t *testing.T) {
	a := newClaude("claude", claudeCfg(t), [2]string{"<!--a-->", "<!--/a-->"}, filepath.Join(t.TempDir(), ".lock"))
	refs, err := a.Sessions()
	if err != nil {
		t.Fatal(err)
	}
	by := map[string]SessionRef{}
	for _, r := range refs {
		by[r.ID] = r
	}
	// transcript id + skip sessions.jsonl
	tr, ok := by["claude:11111111-1111-1111-1111-111111111111"]
	if !ok {
		t.Fatalf("transcript session missing; got %v", ids(refs))
	}
	if tr.Title != "Reorg planning session" {
		t.Errorf("title = %q", tr.Title)
	}
	if tr.Activity < 2 {
		t.Errorf("activity = %d", tr.Activity)
	}
	// tool_use captured
	found := false
	for _, e := range tr.Events {
		if contains(e.Text, "[tool:Bash]") {
			found = true
		}
	}
	if !found {
		t.Error("tool_use not captured in events")
	}
	// sessions.jsonl must NOT appear as a session
	for _, r := range refs {
		if filepath.Base(r.Path) == "sessions.jsonl" {
			t.Error("sessions.jsonl was indexed as a transcript")
		}
	}
	// subagent kind
	if sr, ok := by["claude:agent-22222222-2222-2222-2222-222222222222"]; !ok || sr.Kind != "subagent" {
		t.Errorf("subagent not indexed correctly: %+v", sr)
	}
	// archive ids
	if _, ok := by["archive:2026-09-27--reorg"]; !ok {
		t.Errorf("dated archive id wrong; got %v", ids(refs))
	}
	if r, ok := by["archive:2026--09--handover-notes"]; !ok || r.Kind != "report" {
		t.Errorf("report archive id wrong; got %v", ids(refs))
	}
}

func ids(rs []SessionRef) []string {
	var o []string
	for _, r := range rs {
		o = append(o, r.ID)
	}
	return o
}
func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
