package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	tea "github.com/charmbracelet/bubbletea"

	"kiro-cli-history/internal/session"
	"kiro-cli-history/internal/ui"
)

const version = "1.0.0"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--help", "-h":
			fmt.Print(`kiro-cli-history — Search & browse Kiro CLI conversations

Usage: kiro-cli-history [flags]

Flags:
  --help, -h       Show this help
  --version, -v    Show version

Keyboard:
  /          Search sessions
  Tab        Cycle focus (search → list → preview)
  j/k        Navigate list or scroll preview
  l/Enter    Open preview
  f          Fullscreen preview
  Ctrl+R     Resume session in Kiro CLI
  Ctrl+Y     Copy conversation to clipboard
  Ctrl+E     Export conversation as markdown
  s          Settings
  ?          Help overlay
  Esc        Back / Quit

Config: ~/.config/kiro-cli-history/config.json
`)
			return
		case "--version", "-v":
			fmt.Printf("kiro-cli-history %s\nby Paresh Maheshwari\n", version)
			return
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\nRun with --help for usage.\n", os.Args[1])
			os.Exit(1)
		}
	}

	session.LoadConfig()
	defer session.CloseDB()

	p := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, ok := final.(ui.Model)
	if !ok || m.ResumeResult == nil {
		return
	}

	s := m.ResumeResult
	fmt.Printf("\nResuming: %s\nDirectory: %s\n\n", s.Title, s.Cwd)

	if err := os.Chdir(s.Cwd); err != nil {
		fmt.Fprintf(os.Stderr, "chdir failed: %v\n", err)
		os.Exit(1)
	}
	bin, err := exec.LookPath("kiro-cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kiro-cli not found in PATH\n")
		os.Exit(1)
	}
	if err := syscall.Exec(bin, []string{"kiro-cli", "chat", "--resume"}, os.Environ()); err != nil {
		fmt.Fprintf(os.Stderr, "exec failed: %v\n", err)
		os.Exit(1)
	}
}
