package tui

import (
	"fmt"
	"strings"

	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// FilterType represents a category of filter.
type FilterType int

const (
	FilterDirty         FilterType = iota // uncommitted changes or diverged
	FilterPartial                         // ahead/behind
	FilterSynced                          // fully synced
	FilterNoRemote                        // no remote configured
	FilterAhead                           // has commits to push
	FilterBehind                          // has commits to pull
	FilterNameMismatch                    // folder name != repo name
	filterCount                           // sentinel for iteration
)

// filterMeta holds display info for each filter type.
type filterMeta struct {
	Name  string
	Short string // short label for status line
}

var filterInfo = map[FilterType]filterMeta{
	FilterDirty:        {Name: "Dirty", Short: "dirty"},
	FilterPartial:      {Name: "Partial", Short: "partial"},
	FilterSynced:       {Name: "Synced", Short: "synced"},
	FilterNoRemote:     {Name: "No Remote", Short: "no-remote"},
	FilterAhead:        {Name: "Ahead", Short: "ahead"},
	FilterBehind:       {Name: "Behind", Short: "behind"},
	FilterNameMismatch: {Name: "Name Mismatch", Short: "name≠"},
}

// AllFilterTypes returns all filter types in display order.
func AllFilterTypes() []FilterType {
	types := make([]FilterType, 0, int(filterCount))
	for i := FilterType(0); i < filterCount; i++ {
		types = append(types, i)
	}
	return types
}

// FilterSet tracks which filters are active.
type FilterSet struct {
	active map[FilterType]bool
}

// NewFilterSet creates an empty filter set.
func NewFilterSet() FilterSet {
	return FilterSet{active: make(map[FilterType]bool)}
}

// Toggle flips a filter on/off.
func (fs *FilterSet) Toggle(f FilterType) {
	if fs.active == nil {
		fs.active = make(map[FilterType]bool)
	}
	if fs.active[f] {
		delete(fs.active, f)
	} else {
		fs.active[f] = true
	}
}

// IsEnabled returns whether a specific filter is on.
func (fs *FilterSet) IsEnabled(f FilterType) bool {
	return fs.active[f]
}

// Clear removes all active filters.
func (fs *FilterSet) Clear() {
	fs.active = make(map[FilterType]bool)
}

// IsActive returns true if any filter is enabled.
func (fs *FilterSet) IsActive() bool {
	return len(fs.active) > 0
}

// ActiveCount returns the number of enabled filters.
func (fs *FilterSet) ActiveCount() int {
	return len(fs.active)
}

// ActiveNames returns the short labels of active filters.
func (fs *FilterSet) ActiveNames() string {
	var names []string
	for _, ft := range AllFilterTypes() {
		if fs.active[ft] {
			names = append(names, filterInfo[ft].Short)
		}
	}
	return strings.Join(names, ", ")
}

// matchesNode checks if a git repo node matches any active filter (OR logic).
func (fs *FilterSet) matchesNode(node *scanner.TreeNode) bool {
	if !node.IsGitRepo || node.Status == nil {
		return false
	}
	s := node.Status
	for ft := range fs.active {
		switch ft {
		case FilterDirty:
			if s.SyncState() == git.StateDirty {
				return true
			}
		case FilterPartial:
			if s.SyncState() == git.StatePartial {
				return true
			}
		case FilterSynced:
			if s.SyncState() == git.StateSynced {
				return true
			}
		case FilterNoRemote:
			if s.SyncState() == git.StateNoRemote {
				return true
			}
		case FilterAhead:
			if s.TotalAhead > 0 {
				return true
			}
		case FilterBehind:
			if s.TotalBehind > 0 {
				return true
			}
		case FilterNameMismatch:
			if s.NameMismatch() {
				return true
			}
		}
	}
	return false
}

// Apply filters a flat node list. If no filters are active, returns the input unchanged.
// When filters are active, keeps git repos that match any active filter plus
// ancestor directory nodes needed to preserve tree structure.
func (fs *FilterSet) Apply(nodes []*scanner.TreeNode) []*scanner.TreeNode {
	if !fs.IsActive() {
		return nodes
	}

	// First pass: find all matching repo nodes
	matched := make(map[*scanner.TreeNode]bool)
	for _, node := range nodes {
		if fs.matchesNode(node) {
			matched[node] = true
			// Mark all ancestors as needed
			for p := node.Parent; p != nil; p = p.Parent {
				matched[p] = true
			}
		}
	}

	// Second pass: filter the flat list keeping only matched nodes
	result := make([]*scanner.TreeNode, 0, len(matched))
	for _, node := range nodes {
		if matched[node] {
			result = append(result, node)
		}
	}
	return result
}

// CountMatches counts how many git repos match the active filters.
func (fs *FilterSet) CountMatches(nodes []*scanner.TreeNode) int {
	if !fs.IsActive() {
		return 0
	}
	count := 0
	for _, node := range nodes {
		if fs.matchesNode(node) {
			count++
		}
	}
	return count
}

// renderFilterPanel renders the filter panel overlay.
func renderFilterPanel(fs *FilterSet, selectedIdx int, width, height int) string {
	panelWidth := 30
	if panelWidth > width-4 {
		panelWidth = width - 4
	}

	var lines []string
	lines = append(lines, "")

	for i, ft := range AllFilterTypes() {
		check := "[ ]"
		if fs.IsEnabled(ft) {
			check = "[x]"
		}
		name := filterInfo[ft].Name
		line := fmt.Sprintf("  %s %s", check, name)
		if i == selectedIdx {
			line = styleSelected.Render(line)
		} else if fs.IsEnabled(ft) {
			line = styleGitSynced.Render(line)
		} else {
			line = styleValue.Render(line)
		}
		lines = append(lines, line)
	}

	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  ↑↓ navigate"))
	lines = append(lines, styleAction.Render("  Space")+styleLabel.Render(" toggle  ")+styleAction.Render("c")+styleLabel.Render(" clear"))
	lines = append(lines, styleAction.Render("  Esc")+styleLabel.Render(" close"))

	content := strings.Join(lines, "\n")

	panel := stylePanelBorder.
		Width(panelWidth).
		Render(content)

	// Center the panel
	padLeft := (width - panelWidth - 2) / 2
	if padLeft < 0 {
		padLeft = 0
	}
	padTop := (height - len(lines) - 4) / 2
	if padTop < 0 {
		padTop = 0
	}

	var result strings.Builder
	for i := 0; i < padTop; i++ {
		result.WriteString("\n")
	}
	for _, line := range strings.Split(panel, "\n") {
		result.WriteString(strings.Repeat(" ", padLeft))
		result.WriteString(line)
		result.WriteString("\n")
	}

	return result.String()
}
