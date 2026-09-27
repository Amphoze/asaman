// Package cli dispatches asaman subcommands.
package cli

import (
	"fmt"
	"os"

	"github.com/amphoze/asaman/internal/meta"
	"github.com/amphoze/asaman/internal/sessions"
)

// Version is the build version.
var Version = "0.1.0-dev"

const usage = `asaman — Agentic Session Assist MANagement
usage: asaman <command> [args]

  search <query>     unified search across sessions + memory (all agents)
  sessions           list recent sessions (all agents)
  mem <query>        search memory facts
  feedback <query>   search feedback facts
  index [--check]    regenerate the inline index + MEMORY stub + cache
  doctor             verify invariants (symlinks, size cap, freshness, meta)
  setup --init-project <dir>   scaffold a new AGENTS/ dir (ignores secrets/)
  serve [--port N]   local observatory UI (loopback only)
  import-csess [--from db]     import csess favourites/tags/notes
  version

global: --config <path>  (default: ~/Projects/AGENTS/asaman.toml or $ASAMAN_CONFIG)`

// Run dispatches args and returns a process exit code.
func Run(args []string) int {
	if len(args) == 0 {
		fmt.Println(usage)
		return 0
	}
	cmd := args[0]
	rest := args[1:]
	cfg, _, rest := flagVal(rest, "config")

	switch cmd {
	case "version", "--version", "-v":
		fmt.Println("asaman " + Version)
		return 0
	case "search":
		return runSearch(cfg, rest, sessions.Opts{})
	case "sessions":
		return runSessionsCmd(cfg, rest)
	case "mem":
		return runSearch(cfg, rest, sessions.Opts{MemoryOnly: true})
	case "feedback":
		return runSearch(cfg, rest, sessions.Opts{MemoryOnly: true})
	case "index":
		return runIndex(cfg, rest)
	case "doctor":
		return runDoctor(cfg, rest)
	case "setup":
		return runSetup(rest)
	case "serve":
		return runServe(cfg, rest)
	case "import-csess":
		return runImport(cfg, rest)
	default:
		fmt.Println(usage)
		return 2
	}
}

func runImport(cfgPath string, args []string) int {
	from, _, _ := flagVal(args, "from")
	if from == "" {
		home, _ := os.UserHomeDir()
		from = home + "/.cache/csess/index.db"
	}
	app, err := loadApp(cfgPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import-csess:", err)
		return 1
	}
	rep, err := meta.ImportCsess(from, app.Cfg.MetaStore)
	if err != nil {
		fmt.Fprintln(os.Stderr, "import-csess:", err)
		return 1
	}
	fmt.Printf("import-csess: mapped=%d unmapped=%d\n", rep.Mapped, rep.Unmapped)
	for _, d := range rep.Details {
		fmt.Println("  !", d)
	}
	if rep.Unmapped > 0 {
		return 1 // unmapped rows block csess deletion
	}
	return 0
}
