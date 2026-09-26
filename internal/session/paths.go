package session

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths returns the JSONL sessions dir and the classic-mode SQLite DB path.
//
//	sessions: $KIRO_HOME/sessions/cli, default ~/.kiro/sessions/cli (all OSes)
//	SQLite:   Linux   $XDG_DATA_HOME/kiro-cli/data.sqlite3 (~/.local/share)
//	          macOS   ~/Library/Application Support/kiro-cli/data.sqlite3
//	          Windows %LOCALAPPDATA%\kiro-cli\data.sqlite3
func Paths() (sessionsDir, sqliteDB string) {
	if d := os.Getenv("KIRO_DEMO_DIR"); d != "" {
		return filepath.Join(d, "kiro", "sessions", "cli"),
			filepath.Join(d, "kiro-cli", "data.sqlite3")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ""
	}

	kiroHome := os.Getenv("KIRO_HOME")
	if kiroHome == "" {
		kiroHome = filepath.Join(home, ".kiro")
	}
	sessionsDir = filepath.Join(kiroHome, "sessions", "cli")

	var dataDir string
	switch runtime.GOOS {
	case "darwin":
		dataDir = filepath.Join(home, "Library", "Application Support")
	case "windows":
		dataDir = os.Getenv("LOCALAPPDATA")
		if dataDir == "" {
			dataDir = filepath.Join(home, "AppData", "Local")
		}
	default:
		dataDir = os.Getenv("XDG_DATA_HOME")
		if !filepath.IsAbs(dataDir) {
			dataDir = filepath.Join(home, ".local", "share")
		}
	}
	sqliteDB = filepath.Join(dataDir, "kiro-cli", "data.sqlite3")
	return
}
