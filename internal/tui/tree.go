package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// renderLegend renders the compact legend at the top of the left panel.
func renderLegend() string {
	return styleLabel.Render("  ") +
		styleGitSynced.Render("●✓") + styleLabel.Render("synced ") +
		styleGitPartial.Render("△") + styleLabel.Render("partial ") +
		styleGitDirty.Render("✗") + styleLabel.Render("dirty ") +
		styleGitNoRemote.Render("?") + styleLabel.Render("no remote ") +
		styleNonGit.Render("○") + styleLabel.Render("no git") +
		"\n" +
		styleLabel.Render("  ") +
		styleGitPartial.Render("↑") + styleLabel.Render("push ") +
		styleGitPartial.Render("↓") + styleLabel.Render("pull ") +
		styleGitSynced.Render("+") + styleLabel.Render("staged ") +
		styleGitDirty.Render("~") + styleLabel.Render("unstaged ") +
		styleNonGit.Render("…") + styleLabel.Render("untracked ") +
		styleGitPartial.Render("≠") + styleLabel.Render("name mismatch")
}

// renderTree renders the left panel tree view.
func renderTree(nodes []*scanner.TreeNode, selectedIdx int, width, height int, filterStatus string) string {
	if len(nodes) == 0 {
		if filterStatus != "" {
			return styleAction.Render("  "+filterStatus) + "\n" + styleNonGit.Render("  No matching repos")
		}
		return styleNonGit.Render("  No directories found")
	}

	var lines []string

	// Legend at the top
	legend := renderLegend()
	lines = append(lines, legend)
	lines = append(lines, styleTreePrefix.Render("  "+strings.Repeat("─", width-4)))

	legendLines := 3 // legend + separator + blank line

	// Filter status line
	if filterStatus != "" {
		lines = append(lines, styleAction.Render("  "+filterStatus))
		legendLines++
	}

	treeHeight := height - legendLines - 1 // -1 for possible scroll indicator

	// Center the selected item in the viewport
	startIdx, endIdx := centeredWindow(selectedIdx, len(nodes), treeHeight)

	for i := startIdx; i < endIdx; i++ {
		node := nodes[i]
		line := renderTreeLine(node, i == selectedIdx, width)
		lines = append(lines, line)
	}

	// Scroll indicator
	if endIdx < len(nodes) {
		lines = append(lines, styleTreePrefix.Render(fmt.Sprintf("  ↓ %d more...", len(nodes)-endIdx)))
	}

	return strings.Join(lines, "\n")
}

// renderTreeLine renders a single tree node line.
func renderTreeLine(node *scanner.TreeNode, selected bool, maxWidth int) string {
	prefix := getTreePrefix(node)
	name := node.Name + "/"
	indicators := getIndicators(node)

	// Build the line
	prefixStr := styleTreePrefix.Render(prefix)

	// Apply color to the name based on status
	nameStyle := getNameStyle(node)
	if selected {
		nameStyle = nameStyle.Background(lipgloss.Color("#374151")).Bold(true)
	}
	nameStr := nameStyle.Render(name)

	line := prefixStr + nameStr
	if indicators != "" {
		line += " " + indicators
	}

	// Wrap if needed - but for TUI we'll truncate with ellipsis for cleanliness
	visibleLen := lipgloss.Width(line)
	if visibleLen > maxWidth && maxWidth > 10 {
		// We'll keep it as-is, lipgloss handles overflow
	}

	return line
}

// getTreePrefix returns the tree connector prefix for a node.
func getTreePrefix(node *scanner.TreeNode) string {
	if node.Parent == nil {
		return "  "
	}

	var parts []string
	current := node
	for current.Parent != nil && current.Parent.Parent != nil {
		parent := current.Parent
		isLast := current == parent.Children[len(parent.Children)-1]
		if isLast {
			parts = append([]string{"    "}, parts...)
		} else {
			parts = append([]string{"│   "}, parts...)
		}
		current = parent
	}

	isLast := false
	if node.Parent != nil {
		children := node.Parent.Children
		isLast = node == children[len(children)-1]
	}

	connector := "├── "
	if isLast {
		connector = "└── "
	}

	return "  " + strings.Join(parts, "") + connector
}

// getIndicators returns the status indicator string for a node.
func getIndicators(node *scanner.TreeNode) string {
	if !node.IsGitRepo || node.Status == nil {
		if !node.IsGitRepo {
			return styleNonGit.Render("○")
		}
		return ""
	}

	s := node.Status
	var parts []string

	// Git indicator
	parts = append(parts, "●")

	// Sync status icon
	switch s.SyncState() {
	case git.StateNoRemote:
		parts = append(parts, styleGitNoRemote.Render("?"))
	case git.StateSynced:
		parts = append(parts, styleGitSynced.Render("✓"))
	case git.StatePartial:
		parts = append(parts, styleGitPartial.Render("△"))
	case git.StateDirty:
		parts = append(parts, styleGitDirty.Render("✗"))
	}

	// Push/pull counters
	if s.HasRemote && (s.TotalAhead > 0 || s.TotalBehind > 0) {
		if s.TotalAhead > 0 {
			parts = append(parts, styleGitPartial.Render(fmt.Sprintf("↑%d", s.TotalAhead)))
		}
		if s.TotalBehind > 0 {
			parts = append(parts, styleGitPartial.Render(fmt.Sprintf("↓%d", s.TotalBehind)))
		}
	}

	// Changes indicators
	if s.Staged > 0 {
		parts = append(parts, styleGitSynced.Render(fmt.Sprintf("+%d", s.Staged)))
	}
	if s.Unstaged > 0 {
		parts = append(parts, styleGitDirty.Render(fmt.Sprintf("~%d", s.Unstaged)))
	}
	if s.Untracked > 0 {
		parts = append(parts, styleNonGit.Render(fmt.Sprintf("…%d", s.Untracked)))
	}

	if s.NameMismatch() {
		parts = append(parts, styleGitPartial.Render("≠"))
	}

	return strings.Join(parts, " ")
}

// getNameStyle returns the lipgloss style for a node name based on its git status.
func getNameStyle(node *scanner.TreeNode) lipgloss.Style {
	if !node.IsGitRepo {
		return styleNonGit
	}
	if node.Status == nil {
		return styleNonGit
	}
	switch node.Status.SyncState() {
	case git.StateNoRemote:
		return styleGitNoRemote
	case git.StateSynced:
		return styleGitSynced
	case git.StatePartial:
		return styleGitPartial
	case git.StateDirty:
		return styleGitDirty
	default:
		return styleNonGit
	}
}

// centeredWindow calculates a viewport window that keeps selectedIdx centered.
// Returns (startIdx, endIdx) for the visible slice.
//
// - If the list is shorter than the viewport, show everything.
// - If selected is near the top, pin to top.
// - If selected is near the bottom, pin to bottom.
// - Otherwise, center the selection.
func centeredWindow(selectedIdx, totalItems, viewportHeight int) (int, int) {
	if totalItems <= viewportHeight {
		return 0, totalItems
	}

	half := viewportHeight / 2

	// Try to center the selected item
	startIdx := selectedIdx - half
	endIdx := startIdx + viewportHeight

	// Pin to top
	if startIdx < 0 {
		startIdx = 0
		endIdx = viewportHeight
	}

	// Pin to bottom
	if endIdx > totalItems {
		endIdx = totalItems
		startIdx = endIdx - viewportHeight
	}

	return startIdx, endIdx
}
