package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"

	"kiro-cli-history/internal/session"
)

func RenderSettings(w, h, cursor int) string {
	cfg := session.AppConfig

	// Dynamic width
	bw := 54
	if w < bw+4 {
		bw = w - 4
	}
	if bw < 40 {
		bw = 40
	}
	innerW := bw - 6

	title := lipgloss.NewStyle().
		Bold(true).
		Foreground(lipgloss.Color("62")).
		Align(lipgloss.Center).
		Width(innerW).
		Render("⚙  Settings")

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("236")).
		Render("  " + strings.Repeat("─", innerW-2))

	key := lipgloss.NewStyle().Foreground(lipgloss.Color("230")).Bold(true)
	desc := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	on := lipgloss.NewStyle().Foreground(lipgloss.Color("2")).Bold(true)
	off := lipgloss.NewStyle().Foreground(lipgloss.Color("1")).Bold(true)
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("238"))

	items := []struct {
		k, d string
		v    bool
	}{
		{"1", "Load --classic / --legacy-ui chats", cfg.SQLiteEnabled},
		{"2", "Full-text search in classic chats", cfg.SQLiteIndex},
	}

	viewLabel := "list"
	if cfg.DefaultView == "tree" {
		viewLabel = "tree"
	}

	body := title + "\n" + sep + "\n\n"
	for _, item := range items {
		toggle := off.Render("  ○ OFF")
		if item.v {
			toggle = on.Render("  ● ON ")
		}
		body += fmt.Sprintf("  %s  %s %s\n\n",
			key.Render("["+item.k+"]"),
			desc.Render(item.d),
			toggle,
		)
	}
	body += fmt.Sprintf("  %s  %s %s\n\n",
		key.Render("[3]"),
		desc.Render("Default sidebar view"),
		on.Render("  "+viewLabel),
	)

	body += hint.Render("  Press 1/2/3 to toggle · s/Esc to close") + "\n" +
		hint.Render("  Changes apply on next launch")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(bw).
		Render(body)

	footer := lipgloss.NewStyle().
		Foreground(lipgloss.Color("240")).
		Align(lipgloss.Center).
		Width(bw).
		Render("~/.config/kiro-cli-history/config.json")

	return fillScreen(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box+"\n"+footer), w, h)
}
