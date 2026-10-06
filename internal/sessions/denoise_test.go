package sessions

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/config"
	"github.com/amphoze/asaman/internal/core"
)

func write(t *testing.T, p, s string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Sessions that only carry the rules file or a tool read of it must not
// outrank (or crowd out) the session where the topic was actually discussed.
func TestSearchDenoise(t *testing.T) {
	dir := t.TempDir()
	rules := `# AGENTS.md instructions for /x\n\n<INSTRUCTIONS>\nThe Sketch Store keeps its Supabase backend.\n</INSTRUCTIONS>`

	// codex: injected rules only, then an unrelated prompt.
	for _, n := range []string{"a", "b", "c"} {
		write(t, filepath.Join(dir, "codex", "rollout-0000000"+n+"-0000-0000-0000-000000000000.jsonl"),
			`{"timestamp":"2026-09-20T00:00:00Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"`+rules+`"}]}}`+"\n"+
				`{"timestamp":"2026-09-20T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"restart the croptalk server `+n+`"}]}}`+"\n")
	}
	// claude: a tool result that happens to read the rules file.
	write(t, filepath.Join(dir, "claude", "p", "tool.jsonl"),
		`{"type":"user","timestamp":"2026-09-21T00:00:00Z","message":{"role":"user","content":"check the mail queue"}}`+"\n"+
			`{"type":"user","timestamp":"2026-09-21T00:00:01Z","message":{"role":"user","content":[{"type":"tool_result","content":"12\tThe Sketch Store keeps its Supabase backend."}]}}`+"\n")
	// claude: the real discussion, with an injected reminder and a harness title line.
	write(t, filepath.Join(dir, "claude", "p", "real.jsonl"),
		`{"type":"user","timestamp":"2026-09-17T00:00:00Z","isMeta":true,"message":{"role":"user","content":"<local-command-caveat>Caveat</local-command-caveat>"}}`+"\n"+
			`{"type":"user","timestamp":"2026-09-17T00:00:01Z","message":{"role":"user","content":"build the admin for the sketch store website <system-reminder>The Sketch Store keeps its Supabase backend.</system-reminder>"}}`+"\n"+
			`{"type":"assistant","timestamp":"2026-09-17T00:00:02Z","message":{"role":"assistant","content":[{"type":"text","text":"Building the sketch store studio."}]}}`+"\n")
	// codex: a parent and its guardian review share session_id; the review
	// must neither overwrite the parent nor be searchable itself.
	write(t, filepath.Join(dir, "codex", "rollout-11111111-0000-0000-0000-000000000000.jsonl"),
		`{"timestamp":"2026-09-18T00:00:00Z","type":"session_meta","payload":{"session_id":"11111111-0000-0000-0000-000000000000","id":"11111111-0000-0000-0000-000000000000"}}`+"\n"+
			`{"timestamp":"2026-09-18T00:00:01Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"add razorpay refunds"}]}}`+"\n")
	write(t, filepath.Join(dir, "codex", "rollout-22222222-0000-0000-0000-000000000000.jsonl"),
		`{"timestamp":"2026-09-18T00:00:02Z","type":"session_meta","payload":{"session_id":"11111111-0000-0000-0000-000000000000","id":"22222222-0000-0000-0000-000000000000","parent_thread_id":"11111111-0000-0000-0000-000000000000","thread_source":"guardian_review"}}`+"\n"+
			`{"timestamp":"2026-09-18T00:00:02Z","type":"session_meta","payload":{"session_id":"11111111-0000-0000-0000-000000000000","id":"11111111-0000-0000-0000-000000000000"}}`+"\n"+
			`{"timestamp":"2026-09-18T00:00:03Z","type":"response_item","payload":{"type":"message","role":"user","content":[{"type":"input_text","text":"assess: add razorpay refunds"}]}}`+"\n")
	// archive note: body must be searchable.
	write(t, filepath.Join(dir, "archive", "2026-09-17-atelier.md"), "# Atelier\n\nShipped commission quotes for the sketch store.\n")

	cfg := &config.Config{Agents: map[string]config.AgentCfg{
		"claude": {Adapter: "claude",
			Sessions: []string{filepath.Join(dir, "claude", "*", "*.jsonl")},
			Archive:  []string{filepath.Join(dir, "archive", "*.md")}},
		"codex": {Adapter: "codex", Sessions: []string{filepath.Join(dir, "codex", "*.jsonl")}},
	}}
	ads, _ := adapters.Build(cfg, filepath.Join(dir, ".lock"))
	db, err := core.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.InitSchema(); err != nil {
		t.Fatal(err)
	}
	if err := Reindex(db, ads, nil); err != nil {
		t.Fatal(err)
	}

	hits, err := Search(db, "sketch store", Opts{})
	if err != nil {
		t.Fatal(err)
	}
	if len(hits) != 3 {
		t.Fatalf("want 3 hits (real, archive, tool read), got %d: %+v", len(hits), hits)
	}
	for _, h := range hits {
		if h.Agent == "codex" {
			t.Errorf("injected rules were indexed: %+v", h)
		}
	}
	if last := hits[len(hits)-1]; last.ID != "claude:tool" {
		t.Errorf("tool-output match should rank last, got %+v", hits)
	}
	var real Hit
	for _, h := range hits {
		if h.ID == "claude:real" {
			real = h
		}
	}
	if real.Title != "build the admin for the sketch store website" {
		t.Errorf("title = %q", real.Title)
	}
	if !strings.HasPrefix(real.Started, "2026-09-17") || real.Path == "" {
		t.Errorf("missing date/path: %+v", real)
	}

	hits, _ = Search(db, "razorpay refunds", Opts{})
	if len(hits) != 1 || hits[0].ID != "codex:11111111-0000-0000-0000-000000000000" || hits[0].Kind != "session" {
		t.Errorf("parent/guardian: %+v", hits)
	}

	// Multi-word queries match sessions with the terms close together, not
	// only the exact phrase; nothing here is far enough apart to be loose.
	hits, _ = Search(db, "sketch store admin website", Opts{})
	if len(hits) != 1 || hits[0].ID != "claude:real" || hits[0].Loose {
		t.Errorf("all-terms query: %+v", hits)
	}
}

func TestCollapseIdenticalSnippets(t *testing.T) {
	got := collapse([]Hit{{ID: "1", Snippet: "same  [x] text"}, {ID: "2", Snippet: "Same [x] text"}, {ID: "3", Snippet: "other [x]"}})
	if len(got) != 2 || got[0].ID != "1" || got[0].Similar != 1 {
		t.Errorf("collapse = %+v", got)
	}
}
