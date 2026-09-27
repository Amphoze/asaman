package cli

import (
	"os"
	"path/filepath"

	"github.com/amphoze/asaman/internal/adapters"
	"github.com/amphoze/asaman/internal/config"
	"github.com/amphoze/asaman/internal/core"
	"github.com/amphoze/asaman/internal/memory"
)

// App bundles the loaded config and derived handles for a command run.
type App struct {
	Cfg  *config.Config
	Lock string
}

func defaultConfigPath() string {
	if p := os.Getenv("ASAMAN_CONFIG"); p != "" {
		return p
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Projects", "AGENTS", "asaman.toml")
}

// loadApp loads config from --config/env/default.
func loadApp(cfgPath string) (*App, error) {
	if cfgPath == "" {
		cfgPath = defaultConfigPath()
	}
	c, err := config.Load(cfgPath)
	if err != nil {
		return nil, err
	}
	lock := c.MetaStore + ".lock"
	return &App{Cfg: c, Lock: lock}, nil
}

func (a *App) adapters() ([]adapters.Adapter, error) {
	return adapters.Build(a.Cfg, a.Lock)
}

func (a *App) facts() ([]memory.Fact, error) {
	if a.Cfg.MemoryDir == "" {
		return nil, nil
	}
	return memory.LoadDir(a.Cfg.MemoryDir)
}

func (a *App) openDB() (*core.DB, error) {
	db, err := core.Open(a.Cfg.CacheDB)
	if err != nil {
		return nil, err
	}
	if err := db.InitSchema(); err != nil {
		db.Close()
		return nil, err
	}
	return db, nil
}
