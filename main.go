package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"

	tea "github.com/charmbracelet/bubbletea"

	"kiro-cli-history/internal/session"
	"kiro-cli-history/internal/ui"
	"kiro-cli-history/internal/update"
)

// version is the release version. Release builds set it with
// -ldflags "-X main.version=<tag without v>"; keep it in sync for source builds.
var version = "1.3.0"

const usage = `kiro-cli-history — Search & browse Kiro CLI conversations

Usage:
  kiro-cli-history                 Open the session browser
  kiro-cli-history update          Update to the latest release
  kiro-cli-history update --check  Only check whether an update is available

Flags:
  --help, -h       Show this help
  --version, -v    Show version

Keyboard:
  /          Search sessions
  Tab        Cycle focus (search → list → preview)
  j/k        Navigate list or scroll preview
  l/Enter    Open preview
  v          Toggle list/tree view (tree: h/l collapse/expand)
  f          Fullscreen preview
  Ctrl+R     Resume session in Kiro CLI
  Ctrl+Y     Copy conversation to clipboard
  Ctrl+E     Export conversation as markdown
  s          Settings
  ?          Help overlay
  Esc        Back / Quit

Config: ~/.config/kiro-cli-history/config.json
`

func main() {
	update.CleanupOld()

	if len(os.Args) > 1 {
		os.Exit(runCommand(os.Args[1:]))
	}

	ui.Version = version
	session.LoadConfig()

	p := tea.NewProgram(ui.NewModel(), tea.WithAltScreen())
	final, err := p.Run()
	session.CloseDB()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}

	m, ok := final.(ui.Model)
	if !ok || m.ResumeResult == nil {
		return
	}
	os.Exit(resume(m.ResumeResult))
}

// runCommand handles non-interactive commands and returns the exit code.
func runCommand(args []string) int {
	switch args[0] {
	case "--help", "-h", "help":
		fmt.Print(usage)
		return 0
	case "--version", "-v", "version":
		fmt.Printf("kiro-cli-history %s\nby Paresh Maheshwari\n", version)
		return 0
	case "update":
		checkOnly := false
		for _, a := range args[1:] {
			switch a {
			case "--check", "-c":
				checkOnly = true
			default:
				fmt.Fprintf(os.Stderr, "Unknown option for update: %s\nUsage: kiro-cli-history update [--check]\n", a)
				return 2
			}
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
		defer stop()
		if err := update.New(os.Stdout).Run(ctx, version, checkOnly); err != nil {
			fmt.Fprintf(os.Stderr, "Update failed: %v\n", err)
			return 1
		}
		return 0
	default:
		fmt.Fprintf(os.Stderr, "Unknown command or flag: %s\nRun with --help for usage.\n", args[0])
		return 2
	}
}

// resume starts `kiro-cli chat --resume-id <id>` in the session's directory.
func resume(s *session.Session) int {
	fmt.Printf("\nResuming: %s\nDirectory: %s\n\n", s.Title, s.Cwd)

	if err := os.Chdir(s.Cwd); err != nil {
		fmt.Fprintf(os.Stderr, "chdir failed: %v\n", err)
		return 1
	}
	bin, err := exec.LookPath("kiro-cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kiro-cli not found in PATH\n")
		return 1
	}
	if err := execReplace(bin, []string{"kiro-cli", "chat", "--resume-id", s.SessionID}); err != nil {
		fmt.Fprintf(os.Stderr, "starting kiro-cli failed: %v\n", err)
		return 1
	}
	return 0
}
