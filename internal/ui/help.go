package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

func helpLine(key, desc string) string {
	k := lipgloss.NewStyle().Foreground(lipgloss.Color("230"))
	d := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	return k.Render(key) + d.Render(desc)
}

func RenderHelp(w, h, scroll int) string {
	bw := 56
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
		Render("⌨  kiro-cli-history")

	sep := lipgloss.NewStyle().Foreground(lipgloss.Color("236")).
		Render("  " + strings.Repeat("─", innerW-2))

	section := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("62"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))

	allLines := []string{
		title, sep, "",
		section.Render("  FOCUS"),
		helpLine("    Tab          ", "Cycle focus"),
		helpLine("    Shift+Tab    ", "Cycle backwards"),
		helpLine("    /  Ctrl+F    ", "Jump to search"),
		"",
		section.Render("  LIST"),
		helpLine("    j/k  ↑/↓     ", "Navigate"),
		helpLine("    g / G        ", "First / last"),
		helpLine("    PgDn / PgUp  ", "Jump 10"),
		helpLine("    l / Enter    ", "Open preview"),
		"",
		section.Render("  PREVIEW"),
		helpLine("    j/k          ", "Scroll"),
		helpLine("    d/u          ", "Half-page"),
		helpLine("    Space / PgDn ", "Full page"),
		helpLine("    g/G          ", "Top / bottom"),
		helpLine("    h / Esc      ", "Back to list"),
		"",
		section.Render("  ACTIONS"),
		helpLine("    f            ", "Fullscreen"),
		helpLine("    Ctrl+R       ", "Resume in Kiro"),
		helpLine("    Ctrl+Y       ", "Copy chat"),
		helpLine("    Ctrl+E       ", "Export as markdown"),
		helpLine("    v            ", "Toggle list/tree view"),
		helpLine("    s            ", "Settings"),
		helpLine("    ?            ", "This help"),
		helpLine("    Ctrl+C       ", "Quit"),
		"",
		section.Render("  TIP"),
		dim.Render("    Select text with mouse normally"),
		"",
		section.Render("  MODES"),
		dim.Render("    SEARCH  LIST  PREVIEW  FULL"),
	}

	// Scrollable window
	maxVisible := h - 8 // box border + padding + footer
	if maxVisible < 5 {
		maxVisible = 5
	}

	// Clamp scroll
	maxScroll := len(allLines) - maxVisible
	if maxScroll < 0 {
		maxScroll = 0
	}
	if scroll > maxScroll {
		scroll = maxScroll
	}

	// Slice visible lines
	end := scroll + maxVisible
	if end > len(allLines) {
		end = len(allLines)
	}
	visible := allLines[scroll:end]

	body := strings.Join(visible, "\n")

	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("62")).
		Padding(1, 2).
		Width(bw).
		Render(body)

	// Footer with scroll hint
	hint := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	footer := "? / Esc close"
	if maxScroll > 0 {
		footer += "  ·  j/k scroll"
	}
	footerLine := hint.Align(lipgloss.Center).Width(bw).Render(footer)

	return fillScreen(lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box+"\n"+footerLine), w, h)
}
