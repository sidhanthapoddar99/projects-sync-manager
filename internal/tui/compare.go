package tui

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// CompareView manages the reference file comparison view as a tree.
type CompareView struct {
	result      *reference.CompareResult
	treeRoot    *compareTreeNode
	flatNodes   []*compareTreeNode
	selectedIdx int
	rootPath    string
	headerText  string // custom header (empty = default "Reference Comparison")
}

// compareTreeNode represents a node in the comparison tree.
type compareTreeNode struct {
	Name     string
	Children []*compareTreeNode
	Parent   *compareTreeNode
	Entry    *compareEntry // nil for intermediate directories
	Expanded bool
	Depth    int
}

type compareEntry struct {
	indicator  string // [==], [++], [--], [⇄]
	path       string // reference path (expected)
	actualPath string // actual local path (differs if relocated)
	remoteURL  string
	status     string // "matched", "missing", "extra", "relocated"
	node       *scanner.TreeNode
}

type cloneDoneMsg struct {
	path string
	err  error
}

type cloneAllDoneMsg struct {
	cloned int
	failed int
}

// flattenVisible returns all visible nodes in the compare tree.
func (n *compareTreeNode) flattenVisible() []*compareTreeNode {
	var result []*compareTreeNode
	result = append(result, n)
	if n.Expanded {
		for _, c := range n.Children {
			result = append(result, c.flattenVisible()...)
		}
	}
	return result
}

// hasEntryDescendant returns true if this node or any descendant has a compare entry.
func (n *compareTreeNode) hasEntryDescendant() bool {
	if n.Entry != nil {
		return true
	}
	for _, c := range n.Children {
		if c.hasEntryDescendant() {
			return true
		}
	}
	return false
}

// isLeaf returns true if this node is a repo entry (not a directory container).
func (n *compareTreeNode) isLeaf() bool {
	return n.Entry != nil
}

// aggregateStatus returns a summary status based on children.
// If all children are matched → matched; if any missing → missing; if mixed → mixed.
func (n *compareTreeNode) aggregateStatus() string {
	if n.Entry != nil {
		return n.Entry.status
	}
	hasMissing := false
	hasMatched := false
	hasExtra := false
	hasRelocated := false
	var walk func(node *compareTreeNode)
	walk = func(node *compareTreeNode) {
		if node.Entry != nil {
			switch node.Entry.status {
			case "matched":
				hasMatched = true
			case "missing":
				hasMissing = true
			case "extra":
				hasExtra = true
			case "relocated":
				hasRelocated = true
			}
			return
		}
		for _, c := range node.Children {
			walk(c)
		}
	}
	walk(n)

	count := 0
	result := "mixed"
	if hasMissing {
		count++
		result = "missing"
	}
	if hasMatched {
		count++
		result = "matched"
	}
	if hasExtra {
		count++
		result = "extra"
	}
	if hasRelocated {
		count++
		result = "relocated"
	}
	if count == 1 {
		return result
	}
	return "mixed"
}

func newCompareViewWithHeader(result *reference.CompareResult, rootPath string, header string) *CompareView {
	cv := newCompareView(result, rootPath)
	cv.headerText = header
	return cv
}

func newCompareView(result *reference.CompareResult, rootPath string) *CompareView {
	cv := &CompareView{
		result:   result,
		rootPath: rootPath,
	}

	// Collect all entries
	var entries []compareEntry
	for _, e := range result.Matched {
		entries = append(entries, compareEntry{
			indicator:  "[==]",
			path:       e.RelativePath,
			actualPath: e.ActualPath,
			remoteURL:  e.RemoteURL,
			status:     "matched",
			node:       e.LocalNode,
		})
	}
	for _, e := range result.Missing {
		entries = append(entries, compareEntry{
			indicator: "[--]",
			path:      e.RelativePath,
			remoteURL: e.RemoteURL,
			status:    "missing",
		})
	}
	for _, e := range result.Extra {
		entries = append(entries, compareEntry{
			indicator:  "[++]",
			path:       e.RelativePath,
			actualPath: e.ActualPath,
			remoteURL:  e.RemoteURL,
			status:     "extra",
			node:       e.LocalNode,
		})
	}
	for _, e := range result.Relocated {
		entries = append(entries, compareEntry{
			indicator:  "[⇄]",
			path:       e.RelativePath,
			actualPath: e.ActualPath,
			remoteURL:  e.RemoteURL,
			status:     "relocated",
			node:       e.LocalNode,
		})
	}

	// Build tree from flat paths
	cv.treeRoot = buildCompareTree(entries, filepath.Base(rootPath))
	cv.rebuildFlatList()

	return cv
}

// buildCompareTree builds a tree structure from flat relative paths.
func buildCompareTree(entries []compareEntry, rootName string) *compareTreeNode {
	root := &compareTreeNode{
		Name:     rootName,
		Expanded: true,
		Depth:    0,
	}

	for i := range entries {
		entry := &entries[i]
		parts := strings.Split(entry.path, string(filepath.Separator))

		current := root
		for j, part := range parts {
			isLast := j == len(parts)-1

			if isLast {
				// This is the actual repo entry
				leaf := &compareTreeNode{
					Name:   part,
					Parent: current,
					Entry:  entry,
					Depth:  current.Depth + 1,
				}
				current.Children = append(current.Children, leaf)
			} else {
				// Find or create intermediate directory
				var found *compareTreeNode
				for _, c := range current.Children {
					if c.Name == part && c.Entry == nil {
						found = c
						break
					}
				}
				if found == nil {
					found = &compareTreeNode{
						Name:     part,
						Parent:   current,
						Expanded: true,
						Depth:    current.Depth + 1,
					}
					current.Children = append(current.Children, found)
				}
				current = found
			}
		}
	}

	// Sort children at every level
	sortCompareTree(root)

	return root
}

// sortCompareTree sorts children at every level: directories first, then alphabetical.
func sortCompareTree(node *compareTreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		iDir := !node.Children[i].isLeaf()
		jDir := !node.Children[j].isLeaf()
		if iDir != jDir {
			return iDir // directories first
		}
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})
	for _, c := range node.Children {
		sortCompareTree(c)
	}
}

func (cv *CompareView) rebuildFlatList() {
	if cv.treeRoot == nil {
		cv.flatNodes = nil
		return
	}
	cv.flatNodes = cv.treeRoot.flattenVisible()
	if cv.selectedIdx >= len(cv.flatNodes) {
		cv.selectedIdx = len(cv.flatNodes) - 1
	}
	if cv.selectedIdx < 0 {
		cv.selectedIdx = 0
	}
}

// --- Navigation (mirrors nav.go logic) ---

func (cv *CompareView) navigateUp() {
	if cv.selectedIdx <= 0 || cv.selectedIdx >= len(cv.flatNodes) {
		return
	}
	current := cv.flatNodes[cv.selectedIdx]
	siblings, sibIdx := compareGetSiblings(current)
	if sibIdx > 0 {
		prev := siblings[sibIdx-1]
		for i, n := range cv.flatNodes {
			if n == prev {
				cv.selectedIdx = i
				return
			}
		}
	}
}

func (cv *CompareView) navigateDown() {
	if cv.selectedIdx < 0 || cv.selectedIdx >= len(cv.flatNodes) {
		return
	}
	current := cv.flatNodes[cv.selectedIdx]
	siblings, sibIdx := compareGetSiblings(current)
	if sibIdx < len(siblings)-1 {
		next := siblings[sibIdx+1]
		for i, n := range cv.flatNodes {
			if n == next {
				cv.selectedIdx = i
				return
			}
		}
	}
}

func (cv *CompareView) navigateRight() {
	if cv.selectedIdx < 0 || cv.selectedIdx >= len(cv.flatNodes) {
		return
	}
	node := cv.flatNodes[cv.selectedIdx]
	if node.isLeaf() || len(node.Children) == 0 {
		return
	}
	if !node.Expanded {
		node.Expanded = true
		cv.rebuildFlatList()
		// Select first child
		for i, n := range cv.flatNodes {
			if n == node.Children[0] {
				cv.selectedIdx = i
				return
			}
		}
	} else {
		// Already expanded, select first child
		for i, n := range cv.flatNodes {
			if n == node.Children[0] {
				cv.selectedIdx = i
				return
			}
		}
	}
}

func (cv *CompareView) navigateLeft() {
	if cv.selectedIdx < 0 || cv.selectedIdx >= len(cv.flatNodes) {
		return
	}
	node := cv.flatNodes[cv.selectedIdx]

	// If expanded directory that does NOT contain entry descendants, collapse it.
	// Mirrors normal nav: folders with entries inside cannot be collapsed.
	if node.Expanded && len(node.Children) > 0 && !node.isLeaf() && !node.hasEntryDescendant() {
		node.Expanded = false
		cv.rebuildFlatList()
		return
	}
	// Navigate to parent
	if node.Parent != nil {
		for i, n := range cv.flatNodes {
			if n == node.Parent {
				cv.selectedIdx = i
				return
			}
		}
	}
}

func compareGetSiblings(node *compareTreeNode) ([]*compareTreeNode, int) {
	if node.Parent == nil {
		return []*compareTreeNode{node}, 0
	}
	siblings := node.Parent.Children
	for i, s := range siblings {
		if s == node {
			return siblings, i
		}
	}
	return siblings, 0
}

// --- Rendering ---

// renderCompareLegend renders the color/symbol legend for compare views.
func renderCompareLegend() string {
	return styleLabel.Render("  ") +
		styleMatchedIndicator.Render("[==]") + styleLabel.Render(" matched  ") +
		styleMissingIndicator.Render("[--]") + styleLabel.Render(" missing  ") +
		styleExtraIndicator.Render("[++]") + styleLabel.Render(" extra  ") +
		styleRelocatedIndicator.Render("[⇄]") + styleLabel.Render(" relocated")
}

func (cv *CompareView) renderLeft(width, height int) string {
	var lines []string
	header := "Reference Comparison"
	if cv.headerText != "" {
		header = cv.headerText
	}
	lines = append(lines, styleHeader.Render("  "+header))

	// Legend
	lines = append(lines, renderCompareLegend())

	summary := fmt.Sprintf("  Matched: %d  Missing: %d  Extra: %d",
		len(cv.result.Matched), len(cv.result.Missing), len(cv.result.Extra))
	if len(cv.result.Relocated) > 0 {
		summary += fmt.Sprintf("  Relocated: %d", len(cv.result.Relocated))
	}
	lines = append(lines, styleLabel.Render(summary))
	lines = append(lines, styleTreePrefix.Render("  "+strings.Repeat("─", width-4)))

	legendLines := 5 // header + legend + summary + separator + extra
	treeHeight := height - legendLines

	startIdx, endIdx := centeredWindow(cv.selectedIdx, len(cv.flatNodes), treeHeight)

	for i := startIdx; i < endIdx; i++ {
		node := cv.flatNodes[i]
		line := cv.renderCompareTreeLine(node, i == cv.selectedIdx, width)
		lines = append(lines, line)
	}

	if endIdx < len(cv.flatNodes) {
		lines = append(lines, styleTreePrefix.Render(fmt.Sprintf("  ↓ %d more...", len(cv.flatNodes)-endIdx)))
	}

	return strings.Join(lines, "\n")
}

func (cv *CompareView) renderCompareTreeLine(node *compareTreeNode, selected bool, maxWidth int) string {
	prefix := compareTreePrefix(node)
	name := node.Name

	// Add "/" suffix for directories
	if !node.isLeaf() {
		name += "/"
	}

	prefixStr := styleTreePrefix.Render(prefix)

	// Determine style based on status
	nameStyle := cv.getCompareNameStyle(node)
	if selected {
		nameStyle = nameStyle.Background(lipgloss.Color("#374151")).Bold(true)
	}
	nameStr := nameStyle.Render(name)

	line := prefixStr + nameStr

	// Add indicator for leaf nodes
	if node.isLeaf() && node.Entry != nil {
		var indicator string
		switch node.Entry.status {
		case "matched":
			indicator = styleMatchedIndicator.Render(" [==]")
		case "missing":
			indicator = styleMissingIndicator.Render(" [--]")
		case "extra":
			indicator = styleExtraIndicator.Render(" [++]")
		case "relocated":
			indicator = styleRelocatedIndicator.Render(" [⇄]")
		}
		line += indicator
	}

	return line
}

func (cv *CompareView) getCompareNameStyle(node *compareTreeNode) lipgloss.Style {
	status := node.aggregateStatus()
	switch status {
	case "matched":
		return styleGitSynced
	case "missing":
		return styleGitDirty
	case "extra":
		return styleGitPartial
	case "relocated":
		return styleRelocatedIndicator
	case "mixed":
		return styleGitNoRemote
	default:
		return styleNonGit
	}
}

func compareTreePrefix(node *compareTreeNode) string {
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

func (cv *CompareView) renderRight(width, height int) string {
	if len(cv.flatNodes) == 0 {
		return styleLabel.Render("  No entries to display")
	}

	node := cv.flatNodes[cv.selectedIdx]

	// If it's a directory node, show summary
	if !node.isLeaf() {
		return cv.renderDirectorySummary(node, width)
	}

	e := node.Entry
	var lines []string

	lines = append(lines, styleHeader.Render("  "+e.path))
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Remote: ")+styleInfo.Render(e.remoteURL))
	lines = append(lines, "")

	switch e.status {
	case "matched":
		lines = append(lines, styleGitSynced.Render("  ✓ Present locally and in reference"))
		if e.node != nil && e.node.Status != nil {
			lines = append(lines, "")
			lines = append(lines, styleLabel.Render("  State: ")+renderSyncStateBadge(e.node.Status.SyncState()))
		}
	case "missing":
		lines = append(lines, styleGitDirty.Render("  ✗ Missing locally"))
		lines = append(lines, "")
		lines = append(lines, styleSection.Render("  Actions"))
		lines = append(lines, styleAction.Render("  [Enter] Clone this repository"))
		lines = append(lines, styleAction.Render("  [A] Clone all missing repositories"))
	case "extra":
		lines = append(lines, styleGitPartial.Render("  + Extra (not in reference file)"))
	case "relocated":
		lines = append(lines, styleRelocatedIndicator.Render("  ⇄ Relocated"))
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render("  Expected: ")+styleValue.Render(e.path))
		lines = append(lines, styleLabel.Render("  Found at: ")+styleInfo.Render(e.actualPath))
		if e.node != nil && e.node.Status != nil {
			lines = append(lines, "")
			lines = append(lines, styleLabel.Render("  State: ")+renderSyncStateBadge(e.node.Status.SyncState()))
		}
	}

	return strings.Join(lines, "\n")
}

func (cv *CompareView) renderDirectorySummary(node *compareTreeNode, width int) string {
	var lines []string
	lines = append(lines, styleHeader.Render("  "+node.Name+"/"))
	lines = append(lines, "")

	// Count children by status
	matched, missing, extra, relocated := 0, 0, 0, 0
	var countAll func(n *compareTreeNode)
	countAll = func(n *compareTreeNode) {
		if n.Entry != nil {
			switch n.Entry.status {
			case "matched":
				matched++
			case "missing":
				missing++
			case "extra":
				extra++
			case "relocated":
				relocated++
			}
			return
		}
		for _, c := range n.Children {
			countAll(c)
		}
	}
	countAll(node)

	lines = append(lines, styleLabel.Render("  Directory summary:"))
	lines = append(lines, "")
	if matched > 0 {
		lines = append(lines, styleGitSynced.Render(fmt.Sprintf("  ✓ %d matched", matched)))
	}
	if missing > 0 {
		lines = append(lines, styleGitDirty.Render(fmt.Sprintf("  ✗ %d missing", missing)))
	}
	if extra > 0 {
		lines = append(lines, styleGitPartial.Render(fmt.Sprintf("  + %d extra", extra)))
	}
	if relocated > 0 {
		lines = append(lines, styleRelocatedIndicator.Render(fmt.Sprintf("  ⇄ %d relocated", relocated)))
	}

	return strings.Join(lines, "\n")
}

func renderSyncStateBadge(state git.SyncState) string {
	switch state {
	case git.StateSynced:
		return styleGitSynced.Render("synced")
	case git.StatePartial:
		return styleGitPartial.Render("partially synced")
	case git.StateDirty:
		return styleGitDirty.Render("uncommitted/diverged")
	case git.StateNoRemote:
		return styleGitNoRemote.Render("no remote")
	default:
		return styleNonGit.Render("not a git repo")
	}
}

// markCloned updates a missing entry to matched after a successful clone.
func (cv *CompareView) markCloned(relPath string, node *scanner.TreeNode) {
	var walk func(n *compareTreeNode)
	walk = func(n *compareTreeNode) {
		if n.Entry != nil && n.Entry.path == relPath && n.Entry.status == "missing" {
			n.Entry.status = "matched"
			n.Entry.indicator = "[==]"
			n.Entry.node = node
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(cv.treeRoot)
	cv.rebuildFlatList()
}

// --- Actions ---

func (cv *CompareView) selectedEntry() *compareEntry {
	if cv.selectedIdx >= len(cv.flatNodes) {
		return nil
	}
	return cv.flatNodes[cv.selectedIdx].Entry
}

// walkEntries calls fn for every leaf entry in the compare tree.
func (cv *CompareView) walkEntries(fn func(entry *compareEntry)) {
	var walk func(n *compareTreeNode)
	walk = func(n *compareTreeNode) {
		if n.Entry != nil {
			fn(n.Entry)
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(cv.treeRoot)
}

// handleCloneAllWithProtocol clones all missing repos using SSH or HTTPS.
func (cv *CompareView) handleCloneAllWithProtocol(useSSH bool, rootPath string) tea.Cmd {
	var missing []compareEntry
	cv.walkEntries(func(entry *compareEntry) {
		if entry.status == "missing" {
			missing = append(missing, *entry)
		}
	})

	if len(missing) == 0 {
		return nil
	}
	return func() tea.Msg {
		cloned, failed := 0, 0
		for _, e := range missing {
			cloneURL := e.remoteURL
			if useSSH {
				cloneURL = git.HTTPSToSSH(e.remoteURL)
			}
			targetPath := filepath.Join(rootPath, e.path)
			err := git.CloneRepo(cloneURL, targetPath)
			if err != nil {
				failed++
			} else {
				cloned++
			}
		}
		return cloneAllDoneMsg{cloned: cloned, failed: failed}
	}
}

func (cv *CompareView) handleSyncAll(root *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		var syncAll func(node *compareTreeNode)
		syncAll = func(node *compareTreeNode) {
			if node.Entry != nil && node.Entry.status == "matched" && node.Entry.node != nil && node.Entry.node.Status != nil {
				s := node.Entry.node.Status
				if s.Staged == 0 && s.Unstaged == 0 {
					git.SyncRepo(node.Entry.node.Path)
				}
			}
			for _, c := range node.Children {
				syncAll(c)
			}
		}
		syncAll(cv.treeRoot)
		scanner.RefreshAll(root)
		return refreshDoneMsg{root: root}
	}
}
