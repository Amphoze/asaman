package adapters

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/amphoze/asaman/internal/config"
)

// Review Focus: malformed / partial transcript lines are skipped, never fatal.
func TestMalformedTranscriptLinesSkipped(t *testing.T) {
	dir := t.TempDir()
	proj := filepath.Join(dir, "projects", "p")
	os.MkdirAll(proj, 0o755)
	// good line, garbage line, truncated JSON, another good line
	content := `{"type":"user","timestamp":"2026-09-27T00:00:00Z","message":{"role":"user","content":"hello"}}
this is not json
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","tex
{"type":"assistant","timestamp":"2026-09-27T00:00:01Z","message":{"role":"assistant","content":"world"}}
`
	os.WriteFile(filepath.Join(proj, "44444444-4444-4444-4444-444444444444.jsonl"), []byte(content), 0o644)
	a := newClaude("claude", config.AgentCfg{
		Adapter:  "claude",
		Sessions: []string{filepath.Join(dir, "projects", "*", "*.jsonl")},
	}, [2]string{"a", "b"}, filepath.Join(dir, ".lock"))
	refs, err := a.Sessions()
	if err != nil {
		t.Fatalf("Sessions errored on malformed input: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("want 1 session, got %d", len(refs))
	}
	// the two valid events survived; garbage/truncated skipped
	if refs[0].Activity != 2 {
		t.Errorf("valid events = %d want 2 (malformed skipped)", refs[0].Activity)
	}
}
