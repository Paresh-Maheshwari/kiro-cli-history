package session

import (
	"os"
	"path/filepath"
	"runtime"
)

// Paths returns the JSONL sessions dir and SQLite DB path.
// On Linux: ~/.local/share/kiro-cli/data.sqlite3
// On macOS: ~/Library/Application Support/kiro-cli/data.sqlite3
func Paths() (sessionsDir, sqliteDB string) {
	if d := os.Getenv("KIRO_DEMO_DIR"); d != "" {
		return filepath.Join(d, "kiro", "sessions", "cli"),
			filepath.Join(d, "kiro-cli", "data.sqlite3")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return "", ""
	}
	sessionsDir = filepath.Join(home, ".kiro", "sessions", "cli")

	switch runtime.GOOS {
	case "darwin":
		sqliteDB = filepath.Join(home, "Library", "Application Support", "kiro-cli", "data.sqlite3")
	default:
		sqliteDB = filepath.Join(home, ".local", "share", "kiro-cli", "data.sqlite3")
	}
	return
}
