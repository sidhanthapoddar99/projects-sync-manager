package tui

import (
	"fmt"
	"strings"

	"github.com/sid/psm/internal/scanner"
)

// renderInfo renders the right panel info view for a selected node.
func renderInfo(node *scanner.TreeNode, width, height int) string {
	if node == nil {
		return styleLabel.Render("  Select a directory to view info")
	}

	if !node.IsGitRepo {
		return renderNonGitInfo(node)
	}

	if node.Status == nil {
		return styleLabel.Render("  Loading...")
	}

	var lines []string
	s := node.Status

	// Repository info section
	lines = append(lines, styleHeader.Render("  Repository: "+node.Name))
	lines = append(lines, "")

	if s.HasRemote {
		lines = append(lines, styleLabel.Render("  Remote: ")+styleValue.Render(s.RemoteURL))
		lines = append(lines, styleLabel.Render("  URL:    ")+styleInfo.Render(s.HTTPSURL))
	} else {
		lines = append(lines, styleLabel.Render("  Remote: ")+styleGitNoRemote.Render("none configured"))
	}

	lines = append(lines, styleLabel.Render("  Branch: ")+styleValue.Render(s.CurrentBranch))
	lines = append(lines, "")

	// Branch status section
	if len(s.Branches) > 0 {
		lines = append(lines, styleSection.Render("  Branch Status"))
		lines = append(lines, "")

		// Reserve lines for header (above) + working tree section (below)
		// Header: repo name, blank, remote, url, branch, blank = ~6 lines
		// Working tree: section header, blank, up to 4 lines = ~6 lines
		// Leave some breathing room
		maxBranches := height - len(lines) - 8
		if maxBranches < 3 {
			maxBranches = 3
		}

		branches := s.Branches
		truncated := false
		if len(branches) > maxBranches {
			branches = branches[:maxBranches]
			truncated = true
		}

		for _, b := range branches {
			marker := "  "
			if b.IsCurrent {
				marker = "* "
			}
			branchName := fmt.Sprintf("%-20s", b.Name)
			status := b.StatusLabel()

			branchStyle := styleValue
			switch {
			case !b.IsLocal:
				branchStyle = styleNonGit
			case !b.IsRemote:
				branchStyle = styleGitNoRemote
			case b.Ahead > 0 && b.Behind > 0:
				branchStyle = styleGitDirty
			case b.Ahead > 0 || b.Behind > 0:
				branchStyle = styleGitPartial
			default:
				branchStyle = styleGitSynced
			}

			lines = append(lines, "  "+marker+branchStyle.Render(branchName)+" "+styleLabel.Render(status))
		}
		if truncated {
			lines = append(lines, styleLabel.Render(fmt.Sprintf("  ... %d more branches (Enter for details)", len(s.Branches)-maxBranches)))
		}
		lines = append(lines, "")
	}

	// Current branch details
	lines = append(lines, styleSection.Render("  Working Tree"))
	lines = append(lines, "")
	if s.Staged == 0 && s.Unstaged == 0 && s.Untracked == 0 {
		lines = append(lines, styleGitSynced.Render("  Clean working tree"))
	} else {
		if s.Staged > 0 {
			lines = append(lines, styleGitSynced.Render(fmt.Sprintf("  Staged:    %d file(s)", s.Staged)))
		}
		if s.Unstaged > 0 {
			lines = append(lines, styleGitDirty.Render(fmt.Sprintf("  Unstaged:  %d file(s)", s.Unstaged)))
		}
		if s.Untracked > 0 {
			lines = append(lines, styleNonGit.Render(fmt.Sprintf("  Untracked: %d file(s)", s.Untracked)))
		}
	}

	// Truncate to fit panel height, then pad
	if len(lines) > height-2 {
		lines = lines[:height-2]
	}
	for len(lines) < height-2 {
		lines = append(lines, "")
	}

	return strings.Join(lines, "\n")
}

// renderNonGitInfo renders info for a non-git directory.
func renderNonGitInfo(node *scanner.TreeNode) string {
	var lines []string
	lines = append(lines, styleHeader.Render("  Directory: "+node.Name))
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Path: ")+styleValue.Render(node.Path))
	lines = append(lines, "")
	lines = append(lines, styleNonGit.Render("  Not a git repository"))

	childCount := len(node.Children)
	if childCount > 0 {
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  Contains %d subdirectories", childCount)))
	}

	return strings.Join(lines, "\n")
}

// renderActions renders the actions section at the bottom of the right panel.
func renderActions(node *scanner.TreeNode, width int) string {
	if node == nil {
		return ""
	}

	var lines []string
	lines = append(lines, styleSection.Render("  Actions"))
	lines = append(lines, "")

	if node.IsGitRepo && node.Status != nil {
		s := node.Status

		// Sync action
		if s.HasRemote {
			syncLabel := "  [s] Sync"
			switch {
			case s.Staged > 0 || s.Unstaged > 0:
				syncLabel += styleGitDirty.Render(" (blocked: uncommitted changes)")
			case s.TotalAhead > 0 && s.TotalBehind > 0:
				syncLabel += styleGitDirty.Render(" (blocked: diverged)")
			case s.TotalAhead > 0:
				syncLabel += styleGitPartial.Render(fmt.Sprintf(" (push %d commits)", s.TotalAhead))
			case s.TotalBehind > 0:
				syncLabel += styleGitPartial.Render(fmt.Sprintf(" (pull %d commits)", s.TotalBehind))
			default:
				syncLabel += styleGitSynced.Render(" (up to date)")
			}
			lines = append(lines, styleAction.Render(syncLabel))
		}

		lines = append(lines, styleAction.Render("  [c] Open in VS Code"))
		lines = append(lines, styleAction.Render("  [e] Open in File Explorer"))

		if s.HasRemote {
			lines = append(lines, styleAction.Render("  [b] Open in Browser"))
		}
	} else {
		lines = append(lines, styleAction.Render("  [c] Open in VS Code"))
		lines = append(lines, styleAction.Render("  [e] Open in File Explorer"))
	}

	return strings.Join(lines, "\n")
}
