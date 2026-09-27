package cli

import (
	"fmt"
	"os"
	"strings"
)

// runDoctor verifies invariants and returns non-zero if any fail.
func runDoctor(cfgPath string, args []string) int {
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "doctor:", err)
		return 1
	}
	var problems []string

	// 1. Each agent's context file: if a symlink, its target must resolve.
	for name, ag := range app.Cfg.Agents {
		if ag.ContextFile == "" {
			continue
		}
		fi, err := os.Lstat(ag.ContextFile)
		if err != nil {
			problems = append(problems, fmt.Sprintf("%s context_file missing: %s", name, ag.ContextFile))
			continue
		}
		if fi.Mode()&os.ModeSymlink != 0 {
			if _, err := os.Stat(ag.ContextFile); err != nil {
				problems = append(problems, fmt.Sprintf("%s context_file symlink is broken: %s", name, ag.ContextFile))
			}
		}
	}

	// 2. Canonical size within Codex's ~32KB project-doc cap.
	if app.Cfg.Canonical != "" {
		if fi, err := os.Stat(app.Cfg.Canonical); err == nil && fi.Size() > 32*1024 {
			problems = append(problems, fmt.Sprintf("canonical exceeds 32KB cap: %d bytes", fi.Size()))
		}
	}

	// 3. Index freshness.
	if app.Cfg.Canonical != "" {
		if block, err := app.renderIndexBlock(); err == nil {
			if strings.TrimSpace(block) != app.currentBlock() {
				problems = append(problems, "index is stale (run `asaman index`)")
			}
		}
	}

	// 4. Quarantined meta tail present.
	if app.Cfg.MetaStore != "" {
		if _, err := os.Stat(app.Cfg.MetaStore + ".corrupt"); err == nil {
			problems = append(problems, "meta quarantine present: "+app.Cfg.MetaStore+".corrupt (review)")
		}
	}

	if len(problems) == 0 {
		fmt.Println("doctor: all checks passed")
		return 0
	}
	fmt.Println("doctor: problems found:")
	for _, p := range problems {
		fmt.Println("  ✗", p)
	}
	return 1
}
