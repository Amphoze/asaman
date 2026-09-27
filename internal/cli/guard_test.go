package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeGuardConfig makes a config whose claude context_file is a separate path
// (ctxFile), canonical is AGENTS.md with body `body`. Returns cfg + paths.
func writeGuardConfig(t *testing.T, dir, canonBody, ctxFile string) (cfg, canon string) {
	t.Helper()
	agents := filepath.Join(dir, "AGENTS")
	mem := filepath.Join(agents, "memory")
	os.MkdirAll(mem, 0o755)
	os.MkdirAll(filepath.Join(agents, "asaman"), 0o755)
	os.WriteFile(filepath.Join(mem, "feedback_x.md"),
		[]byte("---\nname: rule_x\ndescription: do x\ntype: feedback\n---\nbody\n"), 0o644)
	canon = filepath.Join(dir, "AGENTS.md")
	os.WriteFile(canon, []byte(canonBody), 0o644)
	cfg = filepath.Join(agents, "asaman.toml")
	toml := `memory_dir="` + mem + `"
canonical="` + canon + `"
index_markers=["<!-- asaman:index -->","<!-- /asaman:index -->"]
meta_store="` + filepath.Join(agents, "asaman", "meta.jsonl") + `"
cache_db="` + filepath.Join(dir, "cache.db") + `"

[agents.claude]
adapter="claude"
context_file="` + ctxFile + `"
memory_mode="native"
`
	os.WriteFile(cfg, []byte(toml), 0o644)
	return
}

// F1(a): index against a canonical lacking markers errors and writes nothing.
func TestIndexRefusesMissingMarkers(t *testing.T) {
	dir := t.TempDir()
	canonBody := "# Rules\nno markers here\n"
	cfg, canon := writeGuardConfig(t, dir, canonBody, filepath.Join(dir, "AGENTS.md"))
	if code := Run([]string{"index", "--config", cfg}); code != 1 {
		t.Fatalf("index w/o markers = %d want 1", code)
	}
	got, _ := os.ReadFile(canon)
	if string(got) != canonBody {
		t.Errorf("canonical was modified despite refusal:\n%s", got)
	}
}

// F1(b): --init inserts markers exactly once (idempotent).
func TestIndexInitBootstrapsMarkersOnce(t *testing.T) {
	dir := t.TempDir()
	cfg, canon := writeGuardConfig(t, dir, "# Rules\n", filepath.Join(dir, "AGENTS.md"))
	if code := Run([]string{"index", "--init", "--config", cfg}); code != 0 {
		t.Fatalf("index --init = %d want 0", code)
	}
	first, _ := os.ReadFile(canon)
	if strings.Count(string(first), "<!-- asaman:index -->") != 1 {
		t.Fatalf("expected exactly one marker block:\n%s", first)
	}
	if !strings.Contains(string(first), "rule_x") {
		t.Errorf("index content not written after --init:\n%s", first)
	}
	if code := Run([]string{"index", "--init", "--config", cfg}); code != 0 {
		t.Fatalf("second index --init = %d want 0", code)
	}
	second, _ := os.ReadFile(canon)
	if strings.Count(string(second), "<!-- asaman:index -->") != 1 {
		t.Errorf("second --init duplicated markers:\n%s", second)
	}
}

// F1(c): canonical+symlink stays in sync; symlink preserved.
func TestIndexKeepsSymlinkInSync(t *testing.T) {
	dir := t.TempDir()
	link := filepath.Join(dir, "CLAUDE.md")
	cfg, canon := writeGuardConfig(t, dir,
		"# Rules\n\n<!-- asaman:index -->\n<!-- /asaman:index -->\n", link)
	if err := os.Symlink(canon, link); err != nil {
		t.Fatal(err)
	}
	if code := Run([]string{"index", "--config", cfg}); code != 0 {
		t.Fatalf("index = %d want 0", code)
	}
	fi, _ := os.Lstat(link)
	if fi.Mode()&os.ModeSymlink == 0 {
		t.Fatal("CLAUDE.md is no longer a symlink")
	}
	a, _ := os.ReadFile(link)
	b, _ := os.ReadFile(canon)
	if string(a) != string(b) || !strings.Contains(string(b), "rule_x") {
		t.Errorf("symlink and canonical diverged or missing content")
	}
}

// F1(d): a non-symlink secondary context file is refused (would desync).
func TestIndexRefusesDesync(t *testing.T) {
	dir := t.TempDir()
	ctx := filepath.Join(dir, "CLAUDE.md")
	cfg, _ := writeGuardConfig(t, dir,
		"# Rules\n\n<!-- asaman:index -->\n<!-- /asaman:index -->\n", ctx)
	// ctx is a REGULAR file, not a symlink to canonical.
	os.WriteFile(ctx, []byte("# separate real file\n"), 0o644)
	if code := Run([]string{"index", "--config", cfg}); code != 1 {
		t.Fatalf("index with desynced context_file = %d want 1", code)
	}
}
