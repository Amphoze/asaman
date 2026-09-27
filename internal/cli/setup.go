package cli

import (
	"fmt"
	"os"
	"path/filepath"
)

const gitignoreTemplate = `# asaman: never track secrets
secrets/
*.secret
*_secret*
.env
.env.*

# asaman runtime
asaman/meta.jsonl.lock
asaman/meta.jsonl.corrupt
`

const tomlTemplate = `memory_dir    = "AGENTS/memory"
canonical     = "AGENTS.md"
index_markers = ["<!-- asaman:index -->", "<!-- /asaman:index -->"]
meta_store    = "AGENTS/asaman/meta.jsonl"
cache_db      = "~/.cache/asaman/index.db"

# [agents.claude] and [agents.codex] blocks go here — see 'asaman setup --add-agent'.
`

// runSetup scaffolds a new AGENTS dir (with a secrets-ignoring .gitignore).
func runSetup(args []string) int {
	dir, initProject, _ := flagVal(args, "init-project")
	if !initProject || dir == "" {
		fmt.Fprintln(os.Stderr, "usage: asaman setup --init-project <dir>")
		return 2
	}
	agentsDir := filepath.Join(dir, "AGENTS")
	for _, sub := range []string{"memory", "asaman", "sessions", "specs"} {
		if err := os.MkdirAll(filepath.Join(agentsDir, sub), 0o755); err != nil {
			fmt.Fprintln(os.Stderr, "setup:", err)
			return 1
		}
	}
	gi := filepath.Join(agentsDir, ".gitignore")
	if err := os.WriteFile(gi, []byte(gitignoreTemplate), 0o644); err != nil {
		fmt.Fprintln(os.Stderr, "setup:", err)
		return 1
	}
	tf := filepath.Join(agentsDir, "asaman.toml")
	if _, err := os.Stat(tf); os.IsNotExist(err) {
		if err := os.WriteFile(tf, []byte(tomlTemplate), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "setup:", err)
			return 1
		}
	}
	fmt.Printf("setup: scaffolded %s (memory/ asaman/ sessions/ specs/), .gitignore ignores secrets/\n", agentsDir)
	return 0
}
