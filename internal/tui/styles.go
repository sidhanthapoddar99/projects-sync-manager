package tui

import "github.com/charmbracelet/lipgloss"

var (
	// Colors
	colorGreen  = lipgloss.Color("#22c55e")
	colorYellow = lipgloss.Color("#eab308")
	colorRed    = lipgloss.Color("#ef4444")
	colorBlue   = lipgloss.Color("#3b82f6")
	colorGrey   = lipgloss.Color("#6b7280")
	colorWhite  = lipgloss.Color("#e5e7eb")
	colorDim    = lipgloss.Color("#9ca3af")
	colorBg     = lipgloss.Color("#1f2937")
	colorBorder = lipgloss.Color("#374151")

	// Tree item styles
	styleGitSynced  = lipgloss.NewStyle().Foreground(colorGreen)
	styleGitPartial = lipgloss.NewStyle().Foreground(colorYellow)
	styleGitDirty   = lipgloss.NewStyle().Foreground(colorRed)
	styleGitNoRemote = lipgloss.NewStyle().Foreground(colorBlue)
	styleNonGit     = lipgloss.NewStyle().Foreground(colorGrey)
	styleSelected   = lipgloss.NewStyle().Background(lipgloss.Color("#374151")).Bold(true)
	styleTreePrefix = lipgloss.NewStyle().Foreground(colorDim)

	// Right panel
	styleHeader    = lipgloss.NewStyle().Bold(true).Foreground(colorWhite)
	styleLabel     = lipgloss.NewStyle().Foreground(colorDim)
	styleValue     = lipgloss.NewStyle().Foreground(colorWhite)
	styleAction    = lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	styleSection   = lipgloss.NewStyle().Bold(true).Foreground(colorYellow).Underline(true)

	// Status bar
	styleStatusBar = lipgloss.NewStyle().
			Background(lipgloss.Color("#111827")).
			Foreground(colorDim).
			Padding(0, 1)

	// Panels
	stylePanelBorder = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(colorBorder)

	// Compare indicators
	styleMatchedIndicator = lipgloss.NewStyle().Foreground(colorGreen)
	styleMissingIndicator = lipgloss.NewStyle().Foreground(colorRed)
	styleExtraIndicator   = lipgloss.NewStyle().Foreground(colorYellow)

	// Messages
	styleSuccess = lipgloss.NewStyle().Foreground(colorGreen)
	styleError   = lipgloss.NewStyle().Foreground(colorRed)
	styleInfo    = lipgloss.NewStyle().Foreground(colorBlue)
)
