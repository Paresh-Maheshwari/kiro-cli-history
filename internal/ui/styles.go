package ui

import "github.com/charmbracelet/lipgloss"

var (
	TitleStyle    = lipgloss.NewStyle().Bold(true)
	DimStyle      = lipgloss.NewStyle().Faint(true)
	CyanStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("6"))
	GreenStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	SelectedStyle = lipgloss.NewStyle().Background(lipgloss.Color("236")).Foreground(lipgloss.Color("252"))
	BorderStyle   = lipgloss.NewStyle().Border(lipgloss.NormalBorder(), false, true, false, false).BorderForeground(lipgloss.Color("240"))
	StatusStyle   = lipgloss.NewStyle().
			Border(lipgloss.NormalBorder(), true, false, false, false).
			BorderForeground(lipgloss.Color("236")).
			Foreground(lipgloss.Color("245"))
	YouLabel  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("6"))
	KiroLabel = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("2"))
	HeaderKey = lipgloss.NewStyle().Bold(true)
	SepStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	SearchBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("240")).
			Padding(0, 1)
	SearchBoxActive = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(0, 1)
	SearchIcon = lipgloss.NewStyle().Foreground(lipgloss.Color("62")).Bold(true)
)
