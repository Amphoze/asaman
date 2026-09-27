package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/amphoze/asaman/internal/context"
	"github.com/amphoze/asaman/internal/memory"
	"github.com/amphoze/asaman/internal/meta"
	"github.com/amphoze/asaman/internal/sessions"
)

// flagVal returns the value of --name in args (--name v or --name=v) and the
// remaining positional args, plus whether a boolean flag --name was present.
func flagVal(args []string, name string) (val string, present bool, rest []string) {
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--"+name:
			present = true
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				val = args[i+1]
				i++
			}
		case strings.HasPrefix(a, "--"+name+"="):
			present = true
			val = strings.TrimPrefix(a, "--"+name+"=")
		default:
			rest = append(rest, a)
		}
	}
	return
}

// renderIndexBlock loads facts and returns the rendered index text.
func (a *App) renderIndexBlock() (string, error) {
	facts, err := a.facts()
	if err != nil {
		return "", err
	}
	return memory.RenderIndex(facts), nil
}

// currentBlock extracts the text currently inside the canonical's markers.
func (a *App) currentBlock() string {
	data, err := os.ReadFile(a.Cfg.Canonical)
	if err != nil {
		return ""
	}
	s := string(data)
	i := strings.Index(s, a.Cfg.IndexMarkers[0])
	j := strings.Index(s, a.Cfg.IndexMarkers[1])
	if i < 0 || j < 0 || j < i {
		return ""
	}
	return strings.TrimSpace(s[i+len(a.Cfg.IndexMarkers[0]) : j])
}

// runIndex regenerates the index block, MEMORY stub, and rebuilds the cache DB.
// With --check it only reports staleness (exit 1 if the on-disk block differs).
func runIndex(cfgPath string, args []string) int {
	_, check, rest := flagVal(args, "check")
	_ = rest
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		return 1
	}
	block, err := app.renderIndexBlock()
	if err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		return 1
	}
	if check {
		if strings.TrimSpace(block) != app.currentBlock() {
			fmt.Println("index: STALE (run `asaman index`)")
			return 1
		}
		fmt.Println("index: fresh")
		return 0
	}
	// Write index into every agent's context file (symlink-preserving).
	ads, _ := app.adapters()
	for _, ad := range ads {
		if ad.ContextPath() == "" {
			continue
		}
		if err := ad.SyncIndex(block); err != nil {
			fmt.Fprintln(os.Stderr, "index sync:", ad.Name(), err)
			return 1
		}
	}
	// MEMORY.md stub for agents that natively load a memory dir.
	stub := []byte(memory.RenderStub())
	for _, ag := range app.Cfg.Agents {
		if ag.MemoryMode == "native" && ag.MemoryDir != "" {
			if _, err := os.Stat(ag.MemoryDir); err == nil {
				_ = context.WriteFileAtomic(ag.MemoryDir+"/MEMORY.md", stub, app.Lock)
			}
		}
	}
	// Rebuild the searchable cache.
	db, err := app.openDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "index:", err)
		return 1
	}
	defer db.Close()
	facts, _ := app.facts()
	if err := sessions.Reindex(db, ads, facts); err != nil {
		fmt.Fprintln(os.Stderr, "index reindex:", err)
		return 1
	}
	fmt.Println("index: regenerated")
	return 0
}

func runSearch(cfgPath string, args []string, o sessions.Opts) int {
	agent, _, rest := flagVal(args, "agent")
	o.Agent = agent
	q := strings.Join(rest, " ")
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search:", err)
		return 1
	}
	db, err := app.openDB()
	if err != nil {
		fmt.Fprintln(os.Stderr, "search:", err)
		return 1
	}
	defer db.Close()
	ads, _ := app.adapters()
	facts, _ := app.facts()
	sessions.Reindex(db, ads, facts)
	hits, err := sessions.Search(db, q, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "search:", err)
		return 1
	}
	for _, h := range hits {
		fmt.Printf("%-8s %-9s %s\n    %s\n", h.Kind, h.Agent, h.Title, oneLine(h.Snippet))
	}
	return 0
}

func runSessionsCmd(cfgPath string, args []string) int {
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "sessions:", err)
		return 1
	}
	ads, _ := app.adapters()
	metaState, _ := meta.Fold(app.Cfg.MetaStore)
	for _, ad := range ads {
		refs, err := ad.Sessions()
		if err != nil {
			continue
		}
		for _, r := range refs {
			star := " "
			if metaState[r.ID].Fav {
				star = "*"
			}
			fmt.Printf("%s %-7s %-10s %s\n", star, r.Agent, truncate(r.Started, 10), r.Title)
		}
	}
	return 0
}

func oneLine(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	return truncate(s, 120)
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
