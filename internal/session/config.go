package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Config holds user preferences.
type Config struct {
	SQLiteEnabled   bool   `json:"sqlite_enabled"`   // load classic mode SQLite sessions
	SQLiteIndex     bool   `json:"sqlite_index"`     // full-text index SQLite content
	DefaultView     string `json:"default_view"`     // "list" or "tree"
	HideSubagents   bool   `json:"hide_subagents"`   // nest subagent sessions under their parent
	CollapseRewinds bool   `json:"collapse_rewinds"` // show only the newest rewind version
}

var DefaultConfig = Config{
	SQLiteEnabled:   true,
	SQLiteIndex:     false,
	DefaultView:     "tree",
	HideSubagents:   true,
	CollapseRewinds: true,
}

var (
	configMu  sync.RWMutex
	appConfig = DefaultConfig
)

// CurrentConfig returns a snapshot safe to pass to background work.
func CurrentConfig() Config {
	configMu.RLock()
	defer configMu.RUnlock()
	return appConfig
}

func SetConfig(cfg Config) {
	configMu.Lock()
	defer configMu.Unlock()
	appConfig = cfg
}

func configPath() string {
	home, _ := os.UserHomeDir()
	if home == "" {
		return ""
	}
	return filepath.Join(home, ".config", "kiro-cli-history", "config.json")
}

// LoadConfig reads config from ~/.config/kiro-cli-history/config.json.
// Creates default config if not found.
func LoadConfig() error {
	SetConfig(DefaultConfig)
	path := configPath()
	if path == "" {
		return fmt.Errorf("cannot locate configuration directory")
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return SaveConfig(DefaultConfig)
	}
	if err != nil {
		return err
	}

	cfg := DefaultConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return fmt.Errorf("read settings: %w", err)
	}
	if cfg.DefaultView != "tree" && cfg.DefaultView != "list" {
		return fmt.Errorf("invalid default view %q", cfg.DefaultView)
	}
	SetConfig(cfg)
	return nil
}

// SaveConfig writes config to disk.
func SaveConfig(cfg Config) error {
	path := configPath()
	if path == "" {
		return fmt.Errorf("cannot locate configuration directory")
	}
	if err := saveConfigFile(path, cfg); err != nil {
		return err
	}
	SetConfig(cfg)
	return nil
}

func saveConfigFile(path string, cfg Config) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".config-*.json")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
