package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeConfig(t *testing.T, dir string) (cfgPath, canonical string) {
	t.Helper()
	agents := filepath.Join(dir, "AGENTS")
	mem := filepath.Join(agents, "memory")
	os.MkdirAll(mem, 0o755)
	os.MkdirAll(filepath.Join(agents, "asaman"), 0o755)
	// one current fact
	os.WriteFile(filepath.Join(mem, "feedback_x.md"), []byte("---\nname: rule_x\ndescription: do x\ntype: feedback\n---\nbody\n"), 0o644)
	canonical = filepath.Join(dir, "AGENTS.md")
	os.WriteFile(canonical, []byte("# Rules\n\n<!-- asaman:index -->\n<!-- /asaman:index -->\n"), 0o644)
	cfgPath = filepath.Join(agents, "asaman.toml")
	toml := `memory_dir="` + mem + `"
canonical="` + canonical + `"
index_markers=["<!-- asaman:index -->","<!-- /asaman:index -->"]
meta_store="` + filepath.Join(agents, "asaman", "meta.jsonl") + `"
cache_db="` + filepath.Join(dir, "cache.db") + `"

[agents.claude]
adapter="claude"
context_file="` + canonical + `"
memory_mode="native"
`
	os.WriteFile(cfgPath, []byte(toml), 0o644)
	return
}

func TestIndexCheckAndRegenerate(t *testing.T) {
	dir := t.TempDir()
	cfg, canonical := writeConfig(t, dir)

	// initially stale → exit 1
	if code := Run([]string{"index", "--check", "--config", cfg}); code != 1 {
		t.Fatalf("index --check on empty block = %d want 1 (stale)", code)
	}
	// regenerate
	if code := Run([]string{"index", "--config", cfg}); code != 0 {
		t.Fatalf("index = %d want 0", code)
	}
	data, _ := os.ReadFile(canonical)
	if !strings.Contains(string(data), "rule_x") {
		t.Errorf("index not written into canonical:\n%s", data)
	}
	// now fresh → exit 0
	if code := Run([]string{"index", "--check", "--config", cfg}); code != 0 {
		t.Fatalf("index --check after regen = %d want 0 (fresh)", code)
	}
}

func TestDoctorFlagsBrokenSymlink(t *testing.T) {
	dir := t.TempDir()
	cfg, _ := writeConfig(t, dir)
	// point claude context_file at a broken symlink by overwriting config's file
	link := filepath.Join(dir, "CLAUDE.md")
	os.Symlink(filepath.Join(dir, "does-not-exist.md"), link)
	// rewrite config context_file to the broken link
	b, _ := os.ReadFile(cfg)
	nc := strings.Replace(string(b), "context_file=\""+filepath.Join(dir, "AGENTS.md")+"\"", "context_file=\""+link+"\"", 1)
	os.WriteFile(cfg, []byte(nc), 0o644)

	if code := Run([]string{"doctor", "--config", cfg}); code != 1 {
		t.Fatalf("doctor with broken symlink = %d want 1", code)
	}
}

func TestSetupWritesSecretsGitignore(t *testing.T) {
	dir := t.TempDir()
	if code := Run([]string{"setup", "--init-project", dir}); code != 0 {
		t.Fatalf("setup = %d want 0", code)
	}
	for _, sub := range []string{"memory", "asaman", "sessions", "specs"} {
		if _, err := os.Stat(filepath.Join(dir, "AGENTS", sub)); err != nil {
			t.Errorf("missing scaffold dir %s", sub)
		}
	}
	gi, err := os.ReadFile(filepath.Join(dir, "AGENTS", ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(gi), "secrets/") {
		t.Errorf(".gitignore does not ignore secrets/:\n%s", gi)
	}
}
