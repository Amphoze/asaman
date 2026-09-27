package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	home, _ := os.UserHomeDir()
	c, err := Load(filepath.Join("..", "..", "testdata", "asaman.toml"))
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if c.IndexMarkers[0] != "<!-- asaman:index -->" || c.IndexMarkers[1] != "<!-- /asaman:index -->" {
		t.Errorf("markers = %v", c.IndexMarkers)
	}
	if c.IndexDensity != "hybrid" {
		t.Errorf("IndexDensity = %q, want default %q", c.IndexDensity, "hybrid")
	}
	wantMem := filepath.Join(home, "Projects/AGENTS/memory")
	if c.MemoryDir != wantMem {
		t.Errorf("MemoryDir = %q want %q", c.MemoryDir, wantMem)
	}
	cl, ok := c.Agents["claude"]
	if !ok {
		t.Fatal("no claude agent")
	}
	if cl.MemoryMode != "native" {
		t.Errorf("claude memory_mode = %q", cl.MemoryMode)
	}
	if len(cl.Sessions) != 2 || strings.Contains(cl.Sessions[0], "~") {
		t.Errorf("claude sessions not expanded: %v", cl.Sessions)
	}
	cx, ok := c.Agents["codex"]
	if !ok {
		t.Fatal("no codex agent")
	}
	// Capital .Codex time-log must be preserved verbatim (case-sensitive).
	if len(cx.TimeLog) != 1 || !strings.Contains(cx.TimeLog[0], "/.Codex/") {
		t.Errorf("codex time_log lost capital .Codex: %v", cx.TimeLog)
	}
	if len(cx.Sessions) != 1 || !strings.HasSuffix(cx.Sessions[0], "/.codex/sessions/**/*.jsonl") {
		t.Errorf("codex sessions wrong: %v", cx.Sessions)
	}
}
