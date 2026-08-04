package tui

import (
	"github.com/charmbracelet/lipgloss"
)

// Color palette
var (
	Primary   = lipgloss.Color("#6366f1") // Indigo
	Secondary = lipgloss.Color("#8b5cf6") // Violet
	Accent    = lipgloss.Color("#10b981") // Emerald
	Danger    = lipgloss.Color("#ef4444") // Red
	Warning   = lipgloss.Color("#f59e0b") // Amber
	Info      = lipgloss.Color("#3b82f6") // Blue
	Dark      = lipgloss.Color("#1e1b4b") // Dark indigo
	Light     = lipgloss.Color("#e0e7ff") // Light indigo
	White     = lipgloss.Color("#ffffff")
	Gray      = lipgloss.Color("#6b7280")
	DarkGray  = lipgloss.Color("#374151")
	Bg        = lipgloss.Color("#0f0f23")
	PanelBg   = lipgloss.Color("#1a1a2e")
)

// Styles
type Styles struct {
	App           lipgloss.Style
	Header        lipgloss.Style
	Footer        lipgloss.Style
	Title         lipgloss.Style
	Subtitle      lipgloss.Style
	MenuItem      lipgloss.Style
	MenuSelected  lipgloss.Style
	MenuDisabled  lipgloss.Style
	Panel         lipgloss.Style
	PanelTitle    lipgloss.Style
	StatusOK      lipgloss.Style
	StatusWarn    lipgloss.Style
	StatusError   lipgloss.Style
	StatusInfo    lipgloss.Style
	Key           lipgloss.Style
	Value         lipgloss.Style
	Hint          lipgloss.Style
	Error         lipgloss.Style
	Success       lipgloss.Style
	Input         lipgloss.Style
	InputFocused  lipgloss.Style
	Dialog        lipgloss.Style
	DialogTitle   lipgloss.Style
	Button        lipgloss.Style
	ButtonFocused lipgloss.Style
	LogOutput     lipgloss.Style
	AsciiArt      lipgloss.Style
}

// DefaultStyles returns the default style set.
func DefaultStyles() Styles {
	return Styles{
		App: lipgloss.NewStyle().
			Background(Bg).
			Foreground(White),

		Header: lipgloss.NewStyle().
			Background(Dark).
			Foreground(White).
			Padding(0, 2).
			Bold(true).
			Width(80),

		Footer: lipgloss.NewStyle().
			Background(Dark).
			Foreground(Gray).
			Padding(0, 2).
			Width(80),

		Title: lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Margin(1, 0),

		Subtitle: lipgloss.NewStyle().
			Foreground(Secondary).
			Margin(0, 0, 1, 0),

		MenuItem: lipgloss.NewStyle().
			Foreground(White).
			Padding(0, 2),

		MenuSelected: lipgloss.NewStyle().
			Foreground(Accent).
			Background(lipgloss.Color("#064e3b")).
			Padding(0, 2).
			Bold(true),

		MenuDisabled: lipgloss.NewStyle().
			Foreground(Gray).
			Padding(0, 2),

		Panel: lipgloss.NewStyle().
			Background(PanelBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Primary).
			Padding(1, 2).
			Width(76),

		PanelTitle: lipgloss.NewStyle().
			Foreground(Primary).
			Bold(true).
			Underline(true),

		StatusOK: lipgloss.NewStyle().
			Foreground(Accent).
			SetString("●"),

		StatusWarn: lipgloss.NewStyle().
			Foreground(Warning).
			SetString("●"),

		StatusError: lipgloss.NewStyle().
			Foreground(Danger).
			SetString("●"),

		StatusInfo: lipgloss.NewStyle().
			Foreground(Info).
			SetString("●"),

		Key: lipgloss.NewStyle().
			Foreground(Secondary).
			Bold(true),

		Value: lipgloss.NewStyle().
			Foreground(White),

		Hint: lipgloss.NewStyle().
			Foreground(Gray).
			Italic(true),

		Error: lipgloss.NewStyle().
			Foreground(Danger).
			Bold(true),

		Success: lipgloss.NewStyle().
			Foreground(Accent).
			Bold(true),

		Input: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Gray).
			Padding(0, 1).
			Width(60),

		InputFocused: lipgloss.NewStyle().
			Border(lipgloss.NormalBorder()).
			BorderForeground(Primary).
			Padding(0, 1).
			Width(60),

		Dialog: lipgloss.NewStyle().
			Background(PanelBg).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(Warning).
			Padding(2, 4).
			Width(60),

		DialogTitle: lipgloss.NewStyle().
			Foreground(Warning).
			Bold(true).
			Margin(0, 0, 1, 0),

		Button: lipgloss.NewStyle().
			Foreground(White).
			Background(DarkGray).
			Padding(0, 3).
			Margin(0, 1),

		ButtonFocused: lipgloss.NewStyle().
			Foreground(White).
			Background(Primary).
			Padding(0, 3).
			Margin(0, 1),

		LogOutput: lipgloss.NewStyle().
			Foreground(lipgloss.Color("#a5b4fc")).
			Background(lipgloss.Color("#0f172a")).
			Padding(1, 2).
			Width(76),

		AsciiArt: lipgloss.NewStyle().
			Foreground(Primary).
			Align(lipgloss.Center).
			Margin(1, 0),
	}
}

// Logo returns the Alcatraz ASCII logo.
func Logo() string {
	return `
    █████╗ ██╗      ██████╗ █████╗ ████████╗██████╗  █████╗ ███████╗
   ██╔══██╗██║     ██╔════╝██╔══██╗╚══██╔══╝██╔══██╗██╔══██╗╚══███╔╝
   ███████║██║     ██║     ███████║   ██║   ██████╔╝███████║  ███╔╝
   ██╔══██║██║     ██║     ██╔══██║   ██║   ██╔══██╗██╔══██║ ███╔╝
   ██║  ██║███████╗╚██████╗██║  ██║   ██║   ██║  ██║██║  ██║███████╗
   ╚═╝  ╚═╝╚══════╝ ╚═════╝╚═╝  ╚═╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═╝╚══════╝
`
}

// LogoSmall returns a compact logo.
func LogoSmall() string {
	return "🔒 Alcatraz"
}
