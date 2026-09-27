// Package config loads asaman.toml and expands ~-relative paths.
package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
)

// AgentCfg describes one agent adapter's source locations and channels.
type AgentCfg struct {
	Adapter     string   `toml:"adapter"`
	Sessions    []string `toml:"sessions"`
	Meta        []string `toml:"meta"`
	TimeLog     []string `toml:"time_log"`
	Archive     []string `toml:"archive"`
	ContextFile string   `toml:"context_file"`
	MemoryDir   string   `toml:"memory_dir"`
	MemoryMode  string   `toml:"memory_mode"`
}

// Config is the parsed asaman.toml.
type Config struct {
	MemoryDir    string              `toml:"memory_dir"`
	Canonical    string              `toml:"canonical"`
	IndexMarkers [2]string           `toml:"index_markers"`
	MetaStore    string              `toml:"meta_store"`
	CacheDB      string              `toml:"cache_db"`
	Agents       map[string]AgentCfg `toml:"agents"`
}

// expand replaces a leading ~ with the user's home directory.
func expand(p string) string {
	if p == "~" {
		if h, err := os.UserHomeDir(); err == nil {
			return h
		}
		return p
	}
	if strings.HasPrefix(p, "~/") {
		if h, err := os.UserHomeDir(); err == nil {
			return filepath.Join(h, p[2:])
		}
	}
	return p
}

func expandAll(ps []string) []string {
	out := make([]string, len(ps))
	for i, p := range ps {
		out[i] = expand(p)
	}
	return out
}

// Load reads and parses the config at path, expanding ~ in every path field.
func Load(path string) (*Config, error) {
	var c Config
	if _, err := toml.DecodeFile(path, &c); err != nil {
		return nil, fmt.Errorf("config: %w", err)
	}
	c.MemoryDir = expand(c.MemoryDir)
	c.Canonical = expand(c.Canonical)
	c.MetaStore = expand(c.MetaStore)
	c.CacheDB = expand(c.CacheDB)
	for name, a := range c.Agents {
		a.Sessions = expandAll(a.Sessions)
		a.Meta = expandAll(a.Meta)
		a.TimeLog = expandAll(a.TimeLog)
		a.Archive = expandAll(a.Archive)
		a.ContextFile = expand(a.ContextFile)
		a.MemoryDir = expand(a.MemoryDir)
		c.Agents[name] = a
	}
	return &c, nil
}
