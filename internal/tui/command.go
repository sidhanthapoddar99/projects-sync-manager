package tui

import (
	"fmt"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/scanner"
)

// Command represents an executable command in the palette.
type Command struct {
	Name    string                        // display name
	Hint    string                        // optional keybinding hint
	Execute func(m *Model) (tea.Model, tea.Cmd) // action to perform
}

// CommandPalette is the fuzzy-search command overlay.
type CommandPalette struct {
	input       string
	commands    []Command
	filtered    []Command
	selectedIdx int
}

type cmdAction int

const (
	cmdActionNone    cmdAction = iota
	cmdActionClose             // user pressed Esc
	cmdActionExecute           // user pressed Enter
)

type cmdResult struct {
	action  cmdAction
	command *Command
}

const maxVisibleCommands = 8

// newCommandPalette builds the command list from the current model state.
func newCommandPalette(m *Model) *CommandPalette {
	cp := &CommandPalette{}
	cp.commands = buildCommands()
	cp.filtered = cp.commands
	return cp
}

func buildCommands() []Command {
	return []Command{
		{Name: "Sync", Hint: "s", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			return m.handleSyncKey()
		}},
		{Name: "Refresh", Hint: "r", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			if m.selectedIdx < len(m.flatNodes) {
				node := m.flatNodes[m.selectedIdx]
				if node.IsGitRepo {
					m.statusText = fmt.Sprintf("Refreshing %s...", node.Name)
					return *m, func() tea.Msg {
						scanner.RefreshNode(node)
						return refreshSingleDoneMsg{node: node}
					}
				}
			}
			return *m, nil
		}},
		{Name: "Refresh All", Hint: "R", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			m.confirmRefreshAll = true
			m.statusText = "Refresh ALL repos from remote? This may take a while. (y/n)"
			m.statusError = false
			return *m, nil
		}},
		{Name: "Open in VS Code", Hint: "c", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			if m.selectedIdx < len(m.flatNodes) {
				return *m, handleOpenVSCode(m.flatNodes[m.selectedIdx])
			}
			return *m, nil
		}},
		{Name: "Open in Explorer", Hint: "e", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			if m.selectedIdx < len(m.flatNodes) {
				return *m, handleOpenExplorer(m.flatNodes[m.selectedIdx])
			}
			return *m, nil
		}},
		{Name: "Open in Browser", Hint: "b", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			return m.handleBrowserKey()
		}},
		{Name: "Reference: Generate", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			return m.handleRefMenuKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'g'}})
		}},
		{Name: "Reference: Load & Compare", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			return m.handleRefMenuKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
		}},
		// Filter toggles
		{Name: "Filter: Toggle Dirty", Execute: filterToggleCmd(FilterDirty)},
		{Name: "Filter: Toggle Partial", Execute: filterToggleCmd(FilterPartial)},
		{Name: "Filter: Toggle Synced", Execute: filterToggleCmd(FilterSynced)},
		{Name: "Filter: Toggle No Remote", Execute: filterToggleCmd(FilterNoRemote)},
		{Name: "Filter: Toggle Ahead", Execute: filterToggleCmd(FilterAhead)},
		{Name: "Filter: Toggle Behind", Execute: filterToggleCmd(FilterBehind)},
		{Name: "Filter: Toggle Name Mismatch", Execute: filterToggleCmd(FilterNameMismatch)},
		{Name: "Filter: Clear All", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			m.filters.Clear()
			m.rebuildFlatList()
			m.statusText = "Filters cleared"
			m.statusError = false
			return *m, nil
		}},
		{Name: "Filter: Open Panel", Hint: "F", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			m.viewMode = ViewFilter
			return *m, nil
		}},
		{Name: "Help", Hint: "?", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			m.viewMode = ViewHelp
			return *m, nil
		}},
		{Name: "Quit", Hint: "q", Execute: func(m *Model) (tea.Model, tea.Cmd) {
			return *m, tea.Quit
		}},
	}
}

func filterToggleCmd(ft FilterType) func(m *Model) (tea.Model, tea.Cmd) {
	return func(m *Model) (tea.Model, tea.Cmd) {
		m.filters.Toggle(ft)
		m.rebuildFlatList()
		if m.filters.IsActive() {
			count := m.filters.CountMatches(m.root.FlattenVisible())
			m.statusText = fmt.Sprintf("Filter: %s (%d repos)", m.filters.ActiveNames(), count)
		} else {
			m.statusText = "Filters cleared"
		}
		m.statusError = false
		return *m, nil
	}
}

// handleKey processes input for the command palette and returns what to do.
func (cp *CommandPalette) handleKey(msg tea.KeyMsg) cmdResult {
	switch msg.Type {
	case tea.KeyEscape:
		return cmdResult{action: cmdActionClose}
	case tea.KeyEnter:
		if cp.selectedIdx < len(cp.filtered) {
			cmd := cp.filtered[cp.selectedIdx]
			return cmdResult{action: cmdActionExecute, command: &cmd}
		}
		return cmdResult{action: cmdActionClose}
	case tea.KeyTab:
		// Auto-complete: populate input with selected command name
		if cp.selectedIdx < len(cp.filtered) {
			cp.input = cp.filtered[cp.selectedIdx].Name
			cp.refilter()
		}
		return cmdResult{action: cmdActionNone}
	case tea.KeyBackspace:
		if len(cp.input) > 0 {
			_, size := utf8.DecodeLastRuneInString(cp.input)
			cp.input = cp.input[:len(cp.input)-size]
			cp.refilter()
		}
		return cmdResult{action: cmdActionNone}
	case tea.KeyUp:
		if cp.selectedIdx > 0 {
			cp.selectedIdx--
		}
		return cmdResult{action: cmdActionNone}
	case tea.KeyDown:
		if cp.selectedIdx < len(cp.filtered)-1 {
			cp.selectedIdx++
		}
		return cmdResult{action: cmdActionNone}
	case tea.KeyRunes:
		cp.input += string(msg.Runes)
		cp.refilter()
		return cmdResult{action: cmdActionNone}
	}
	return cmdResult{action: cmdActionNone}
}

// refilter applies fuzzy matching and resets selection.
func (cp *CommandPalette) refilter() {
	if cp.input == "" {
		cp.filtered = cp.commands
		cp.selectedIdx = 0
		return
	}

	type scored struct {
		cmd   Command
		score int
	}
	var matches []scored
	for _, cmd := range cp.commands {
		if score := fuzzyMatch(cmd.Name, cp.input); score > 0 {
			matches = append(matches, scored{cmd, score})
		}
	}

	// Sort by score descending (simple insertion sort — small list)
	for i := 1; i < len(matches); i++ {
		for j := i; j > 0 && matches[j].score > matches[j-1].score; j-- {
			matches[j], matches[j-1] = matches[j-1], matches[j]
		}
	}

	cp.filtered = make([]Command, len(matches))
	for i, m := range matches {
		cp.filtered[i] = m.cmd
	}
	cp.selectedIdx = 0
}

// fuzzyMatch checks if all characters of pattern appear in order in str.
// Returns a score (higher is better) or 0 for no match.
func fuzzyMatch(str, pattern string) int {
	str = strings.ToLower(str)
	pattern = strings.ToLower(pattern)

	pIdx := 0
	score := 0
	consecutive := 0

	for i := 0; i < len(str) && pIdx < len(pattern); i++ {
		if str[i] == pattern[pIdx] {
			pIdx++
			consecutive++
			score += consecutive * 2 // reward consecutive matches
			if i == 0 || str[i-1] == ' ' || str[i-1] == ':' {
				score += 5 // reward word boundary matches
			}
		} else {
			consecutive = 0
		}
	}

	if pIdx == len(pattern) {
		return score
	}
	return 0 // not all pattern chars matched
}

// render draws the command palette overlay.
func (cp *CommandPalette) render(width, height int) string {
	paletteWidth := width / 2
	if paletteWidth < 40 {
		paletteWidth = 40
	}
	if paletteWidth > width-4 {
		paletteWidth = width - 4
	}

	var lines []string

	// Input line
	inputLine := styleAction.Render(" > ") + styleValue.Render(cp.input) + styleLabel.Render("│")
	lines = append(lines, inputLine)
	lines = append(lines, styleTreePrefix.Render(" "+strings.Repeat("─", paletteWidth-4)))

	// Command list — windowed around selectedIdx
	startIdx, endIdx := centeredWindow(cp.selectedIdx, len(cp.filtered), maxVisibleCommands)
	visible := cp.filtered[startIdx:endIdx]

	for idx, cmd := range visible {
		i := startIdx + idx
		name := cmd.Name
		hint := ""
		if cmd.Hint != "" {
			hint = "[" + cmd.Hint + "]"
		}

		// Pad name and right-align hint
		nameWidth := paletteWidth - 6 - len(hint)
		if nameWidth < 0 {
			nameWidth = 0
		}
		if len(name) > nameWidth {
			name = name[:nameWidth]
		}
		padding := nameWidth - len(name)
		if padding < 0 {
			padding = 0
		}

		line := "  " + name + strings.Repeat(" ", padding) + " " + hint
		if i == cp.selectedIdx {
			line = styleSelected.Render(line)
		} else {
			line = styleValue.Render(line)
		}
		lines = append(lines, line)
	}

	if endIdx < len(cp.filtered) {
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  ↓ %d more...", len(cp.filtered)-endIdx)))
	}

	if len(cp.filtered) == 0 {
		lines = append(lines, styleLabel.Render("  No matching commands"))
	}

	content := strings.Join(lines, "\n")

	panel := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(lipgloss.Color("#3b82f6")).
		Width(paletteWidth).
		Render(content)

	// Position at top center
	padLeft := (width - paletteWidth - 2) / 2
	if padLeft < 0 {
		padLeft = 0
	}

	var result strings.Builder
	result.WriteString("\n") // small top margin
	for _, line := range strings.Split(panel, "\n") {
		result.WriteString(strings.Repeat(" ", padLeft))
		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String()
}
