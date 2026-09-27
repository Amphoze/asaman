// Package adapters turns each agent's on-disk transcripts into a common shape.
package adapters

import "github.com/amphoze/asaman/internal/config"

// MemoryMode describes how an agent recalls memory facts.
type MemoryMode string

const (
	ModeNative      MemoryMode = "native"       // agent surfaces files itself (Claude)
	ModeInlineIndex MemoryMode = "inline-index" // relies on the inline index + asaman (Codex)
	ModeNone        MemoryMode = "none"
)

// Event is one message/tool event within a session.
type Event struct {
	Idx  int
	Role string // user | assistant | tool | tool_result | summary
	Ts   string
	Text string
}

// SessionRef is one indexed session.
type SessionRef struct {
	ID       string // {agent}:{uuid} or archive:{stem}
	Agent    string
	Title    string
	Started  string
	Path     string
	Kind     string // session | subagent | report
	Activity int    // event count (proxy for size on the sky ribbon)
	Events   []Event
}

// Adapter is one agent's integration.
type Adapter interface {
	Name() string
	Sessions() ([]SessionRef, error)
	TimeLog() []string
	ContextPath() string
	MemoryMode() MemoryMode
	SyncIndex(index string) error
}

// Factory builds an adapter from its config block.
type Factory func(name string, cfg config.AgentCfg, markers [2]string, lockPath string) Adapter

var registry = map[string]Factory{}

// Register makes an adapter kind available by its `adapter` config value.
func Register(kind string, f Factory) { registry[kind] = f }

// Build constructs all configured adapters.
func Build(cfg *config.Config, lockPath string) ([]Adapter, error) {
	var out []Adapter
	names := make([]string, 0, len(cfg.Agents))
	for n := range cfg.Agents {
		names = append(names, n)
	}
	// stable order: claude before codex etc.
	for _, n := range sortedNames(names) {
		a := cfg.Agents[n]
		f, ok := registry[a.Adapter]
		if !ok {
			continue
		}
		out = append(out, f(n, a, cfg.IndexMarkers, lockPath))
	}
	return out, nil
}

func sortedNames(ns []string) []string {
	for i := 0; i < len(ns); i++ {
		for j := i + 1; j < len(ns); j++ {
			if ns[j] < ns[i] {
				ns[i], ns[j] = ns[j], ns[i]
			}
		}
	}
	return ns
}
