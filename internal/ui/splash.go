package ui

import (
	"github.com/charmbracelet/lipgloss"
)

var (
	splashLogo = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62")).
			Bold(true)

	splashSub = lipgloss.NewStyle().
			Foreground(lipgloss.Color("245"))

	splashVer = lipgloss.NewStyle().
			Foreground(lipgloss.Color("62"))

	splashCredit = lipgloss.NewStyle().
			Foreground(lipgloss.Color("240"))

	splashCreditName = lipgloss.NewStyle().
				Foreground(lipgloss.Color("62")).
				Bold(true)

	splashBox = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("62")).
			Padding(1, 3)

	center = func(w int) lipgloss.Style {
		return lipgloss.NewStyle().Align(lipgloss.Center).Width(w)
	}
)

const asciiLogo = `
 ██╗  ██╗██╗██████╗  ██████╗ 
 ██║ ██╔╝██║██╔══██╗██╔═══██╗
 █████╔╝ ██║██████╔╝██║   ██║
 ██╔═██╗ ██║██╔══██╗██║   ██║
 ██║  ██╗██║██║  ██║╚██████╔╝
 ╚═╝  ╚═╝╚═╝╚═╝  ╚═╝ ╚═════╝`

func RenderSplash(w, h int, spinnerView string) string {
	if w == 0 || h == 0 {
		return "Loading..."
	}

	cw := 40 // content width
	c := center(cw)

	logo := splashLogo.Render(asciiLogo)
	title := c.Render(splashSub.Render("─── ") + splashVer.Render("CLI HISTORY v1.1.0") + splashSub.Render(" ───"))
	tagline := c.Render(splashSub.Render("Search · Browse · Resume"))
	loading := c.Render(spinnerView + splashSub.Render(" Loading sessions..."))
	credit := c.Render(splashCredit.Render("crafted by ") + splashCreditName.Render("Paresh Maheshwari"))

	inner := logo + "\n" +
		title + "\n\n" +
		tagline + "\n\n" +
		loading + "\n\n" +
		credit

	box := splashBox.Render(inner)

	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, box)
}
