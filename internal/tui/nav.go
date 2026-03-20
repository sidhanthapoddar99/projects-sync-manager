package tui

import "github.com/sid/psm/internal/scanner"

// getSiblings returns the sibling nodes (same parent) and the index of the given node among them.
func getSiblings(node *scanner.TreeNode) ([]*scanner.TreeNode, int) {
	if node.Parent == nil {
		return []*scanner.TreeNode{node}, 0
	}
	siblings := node.Parent.Children
	for i, s := range siblings {
		if s == node {
			return siblings, i
		}
	}
	return siblings, 0
}

// navigateUp moves to the previous sibling at the same directory level.
// When filtered is true, moves sequentially through the flat list instead.
// Returns the new selectedIdx in the flatNodes list.
func navigateUp(flatNodes []*scanner.TreeNode, selectedIdx int, filtered bool) int {
	if selectedIdx <= 0 || selectedIdx >= len(flatNodes) {
		return selectedIdx
	}

	if filtered {
		// Skip directory nodes — only land on git repos
		for i := selectedIdx - 1; i >= 0; i-- {
			if flatNodes[i].IsGitRepo {
				return i
			}
		}
		return selectedIdx
	}

	current := flatNodes[selectedIdx]
	siblings, sibIdx := getSiblings(current)

	if sibIdx > 0 {
		// Move to previous sibling
		prevSibling := siblings[sibIdx-1]
		for i, n := range flatNodes {
			if n == prevSibling {
				return i
			}
		}
	}
	return selectedIdx
}

// navigateDown moves to the next sibling at the same directory level.
// When filtered is true, moves sequentially through the flat list instead.
// Returns the new selectedIdx in the flatNodes list.
func navigateDown(flatNodes []*scanner.TreeNode, selectedIdx int, filtered bool) int {
	if selectedIdx < 0 || selectedIdx >= len(flatNodes) {
		return selectedIdx
	}

	if filtered {
		// Skip directory nodes — only land on git repos
		for i := selectedIdx + 1; i < len(flatNodes); i++ {
			if flatNodes[i].IsGitRepo {
				return i
			}
		}
		return selectedIdx
	}

	current := flatNodes[selectedIdx]
	siblings, sibIdx := getSiblings(current)

	if sibIdx < len(siblings)-1 {
		// Move to next sibling
		nextSibling := siblings[sibIdx+1]
		for i, n := range flatNodes {
			if n == nextSibling {
				return i
			}
		}
	}
	return selectedIdx
}

// navigateRight enters a directory (expands it and selects first child).
// Returns new selectedIdx and whether anything changed.
func navigateRight(flatNodes []*scanner.TreeNode, selectedIdx int) (int, bool) {
	if selectedIdx < 0 || selectedIdx >= len(flatNodes) {
		return selectedIdx, false
	}

	node := flatNodes[selectedIdx]

	// If it's a git repo, don't enter (it's a leaf)
	if node.IsGitRepo {
		return selectedIdx, false
	}

	// If it has no children, try to lazy-expand (scan subdirectories)
	if len(node.Children) == 0 {
		scanner.ExpandNode(node)
	}

	if len(node.Children) == 0 {
		return selectedIdx, false
	}

	// Expand and move to first child
	if !node.Expanded {
		node.Expanded = true
		return selectedIdx, true // caller should rebuild flat list then select first child
	}

	// Already expanded — select first child
	firstChild := node.Children[0]
	for i, n := range flatNodes {
		if n == firstChild {
			return i, false
		}
	}

	return selectedIdx, false
}

// navigateLeft collapses the current directory or moves to parent.
// Folders that contain git repos (directly or nested) cannot be collapsed.
// Returns new selectedIdx and whether the tree structure changed.
func navigateLeft(flatNodes []*scanner.TreeNode, selectedIdx int) (int, bool) {
	if selectedIdx < 0 || selectedIdx >= len(flatNodes) {
		return selectedIdx, false
	}

	node := flatNodes[selectedIdx]

	// If this node is expanded AND does NOT contain git repos, collapse it
	if node.Expanded && len(node.Children) > 0 && !node.HasGitDescendant() {
		node.Expanded = false
		return selectedIdx, true // tree changed, caller rebuilds
	}

	// Otherwise navigate to parent
	if node.Parent != nil {
		for i, n := range flatNodes {
			if n == node.Parent {
				return i, false
			}
		}
	}

	return selectedIdx, false
}
