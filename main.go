package main

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"

	tea "github.com/charmbracelet/bubbletea"

	"kiro-cli-history/internal/session"
	"kiro-cli-history/internal/terminaltext"
	"kiro-cli-history/internal/ui"
	"kiro-cli-history/internal/update"
)

// version is the release version. Release builds set it with
// -ldflags "-X main.version=<tag without v>"; keep it in sync for source builds.
var version = "1.5.0"

const usage = `kiro-cli-history — Search & browse Kiro CLI conversations

Usage:
  kiro-cli-history                 Open the session browser (all directories)
  kiro-cli-history --here          Only sessions from the current directory
  kiro-cli-history --cwd DIR       Only sessions from DIR
  kiro-cli-history update          Update to the latest release
  kiro-cli-history update --check  Only check whether an update is available

Flags:
  --here           Show only sessions started in the current directory (fast)
  --cwd DIR        Show only sessions started in DIR
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
		// update/help/version are commands; anything else is a browser flag.
		switch os.Args[1] {
		case "update", "--help", "-h", "help", "--version", "-v", "version":
			os.Exit(runCommand(os.Args[1:]))
		}
	}

	if code, ok := parseBrowserFlags(os.Args[1:]); !ok {
		os.Exit(code)
	}

	ui.Version = version
	configErr := session.LoadConfig()
	model := ui.NewModel()
	if configErr != nil {
		model.SetNote(fmt.Sprintf("Settings: %v (using defaults)", configErr))
	}
	p := tea.NewProgram(model, tea.WithAltScreen())
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

// parseBrowserFlags sets ui.Filter from the browser flags. It returns ok=false
// with an exit code when the program should stop (bad flag, or --cwd resolved).
func parseBrowserFlags(args []string) (int, bool) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--here", "-H":
			cwd, err := os.Getwd()
			if err != nil {
				fmt.Fprintf(os.Stderr, "cannot determine current directory: %v\n", err)
				return 1, false
			}
			ui.Filter.Cwd = cwd
		case "--cwd":
			if i+1 >= len(args) {
				fmt.Fprintf(os.Stderr, "--cwd needs a directory\n")
				return 2, false
			}
			i++
			abs, err := filepath.Abs(args[i])
			if err != nil {
				fmt.Fprintf(os.Stderr, "invalid --cwd path: %v\n", err)
				return 2, false
			}
			ui.Filter.Cwd = abs
		default:
			fmt.Fprintf(os.Stderr, "Unknown flag: %s\nRun with --help for usage.\n", args[i])
			return 2, false
		}
	}
	return 0, true
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
	if s.SessionID == "" {
		fmt.Fprintln(os.Stderr, "This legacy conversation has no resumable session ID")
		return 1
	}
	fmt.Printf("\nResuming: %s\nDirectory: %s\n\n", terminaltext.Line(s.Title), terminaltext.Line(s.Cwd))

	if err := os.Chdir(s.Cwd); err != nil {
		fmt.Fprintf(os.Stderr, "chdir failed: %s\n", terminaltext.Line(err.Error()))
		return 1
	}
	bin, err := exec.LookPath("kiro-cli")
	if err != nil {
		fmt.Fprintf(os.Stderr, "kiro-cli not found in PATH\n")
		return 1
	}
	if err := execReplace(bin, resumeArgs(s)); err != nil {
		fmt.Fprintf(os.Stderr, "starting kiro-cli failed: %s\n", terminaltext.Line(err.Error()))
		return 1
	}
	return 0
}

func resumeArgs(s *session.Session) []string {
	args := []string{"kiro-cli", "chat"}
	if s.Source == "jsonl_v3" {
		args = append(args, "--v3")
	}
	return append(args, "--resume-id", s.SessionID)
}
