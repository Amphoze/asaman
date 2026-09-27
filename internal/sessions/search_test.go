package sessions

import (
	"path/filepath"
	"testing"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/config"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/memory"
)

func TestUnifiedSearchAcrossAgents(t *testing.T) {
	db, err := core.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InitSchema(); err != nil {
		t.Fatal(err)
	}

	base, _ := filepath.Abs(filepath.Join("..", "..", "testdata"))
	cfg := &config.Config{
		IndexMarkers: [2]string{"<!--a-->", "<!--/a-->"},
		Agents: map[string]config.AgentCfg{
			"claude": {
				Adapter:  "claude",
				Sessions: []string{filepath.Join(base, "claude", "projects", "*", "*.jsonl")},
			},
			"codex": {
				Adapter:  "codex",
				Sessions: []string{filepath.Join(base, "codex", "sessions", "**", "*.jsonl")},
				Meta:     []string{filepath.Join(base, "codex", "meta", "session_index.jsonl")},
			},
		},
	}
	ads, _ := adapters.Build(cfg, filepath.Join(t.TempDir(), ".lock"))
	facts, _ := memory.LoadDir(filepath.Join(base, "memory"))

	if err := Reindex(db, ads, facts); err != nil {
		t.Fatal(err)
	}

	// Claude session content
	hits, err := Search(db, "reorganize", Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if !hasAgent(hits, "claude") {
		t.Errorf("no claude hit for 'reorganize': %v", hits)
	}
	// Codex session content (tool/assistant)
	hits, _ = Search(db, "mail admin portal", Opts{})
	if !hasAgent(hits, "codex") {
		t.Errorf("no codex hit for 'mail admin portal': %v", hits)
	}
	// Memory fact
	hits, _ = Search(db, "guess", Opts{MemoryOnly: true})
	if len(hits) == 0 || hits[0].Kind != "memory" {
		t.Errorf("memory search failed: %v", hits)
	}
	// Agent filter
	hits, _ = Search(db, "deploy", Opts{Agent: "codex"})
	for _, h := range hits {
		if h.Agent != "codex" {
			t.Errorf("agent filter leaked: %v", h)
		}
	}
}

func hasAgent(hits []Hit, agent string) bool {
	for _, h := range hits {
		if h.Agent == agent {
			return true
		}
	}
	return false
}
