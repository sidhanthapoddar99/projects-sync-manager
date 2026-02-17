package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// DetailView manages the interactive repo detail panel.
type DetailView struct {
	node        *scanner.TreeNode
	selectedIdx int // selected item in the right panel (0=actions area, then branches)
	items       []detailItem
}

type detailItem struct {
	kind   string // "action", "branch", "header", "separator"
	label  string
	branch *git.BranchStatus
	action string // key identifier for actions
}

// refreshSingleDoneMsg signals a single repo refresh completed.
type refreshSingleDoneMsg struct {
	node *scanner.TreeNode
}

func newDetailView(node *scanner.TreeNode) *DetailView {
	dv := &DetailView{node: node}
	dv.rebuild()
	return dv
}

func (dv *DetailView) rebuild() {
	dv.items = nil
	s := dv.node.Status
	if s == nil {
		return
	}

	// Actions section
	dv.items = append(dv.items, detailItem{kind: "header", label: "Actions"})
	dv.items = append(dv.items, detailItem{kind: "action", label: "Refresh this repo (fetch remote)", action: "refresh"})

	if s.HasRemote {
		syncLabel := "Sync (pull/push)"
		switch {
		case s.Staged > 0 || s.Unstaged > 0:
			syncLabel += " — blocked: uncommitted changes"
		case s.TotalAhead > 0 && s.TotalBehind > 0:
			syncLabel += " — blocked: diverged"
		case s.TotalAhead > 0:
			syncLabel += fmt.Sprintf(" — push %d commit(s)", s.TotalAhead)
		case s.TotalBehind > 0:
			syncLabel += fmt.Sprintf(" — pull %d commit(s)", s.TotalBehind)
		default:
			syncLabel += " — up to date"
		}
		dv.items = append(dv.items, detailItem{kind: "action", label: syncLabel, action: "sync"})
	}

	dv.items = append(dv.items, detailItem{kind: "action", label: "Open in VS Code", action: "code"})
	dv.items = append(dv.items, detailItem{kind: "action", label: "Open in File Explorer", action: "explorer"})

	if s.HasRemote {
		dv.items = append(dv.items, detailItem{kind: "action", label: "Open in Browser", action: "browser"})
	}

	// Branch section
	if len(s.Branches) > 0 {
		dv.items = append(dv.items, detailItem{kind: "separator"})
		dv.items = append(dv.items, detailItem{kind: "header", label: "Branches"})

		for i := range s.Branches {
			b := &s.Branches[i]
			dv.items = append(dv.items, detailItem{kind: "branch", branch: b})
		}
	}

	// Working tree section
	dv.items = append(dv.items, detailItem{kind: "separator"})
	dv.items = append(dv.items, detailItem{kind: "header", label: "Working Tree"})
	if s.Staged == 0 && s.Unstaged == 0 && s.Untracked == 0 {
		dv.items = append(dv.items, detailItem{kind: "header", label: "  Clean"})
	} else {
		if s.Staged > 0 {
			dv.items = append(dv.items, detailItem{kind: "header", label: fmt.Sprintf("  Staged: %d file(s)", s.Staged)})
		}
		if s.Unstaged > 0 {
			dv.items = append(dv.items, detailItem{kind: "header", label: fmt.Sprintf("  Unstaged: %d file(s)", s.Unstaged)})
		}
		if s.Untracked > 0 {
			dv.items = append(dv.items, detailItem{kind: "header", label: fmt.Sprintf("  Untracked: %d file(s)", s.Untracked)})
		}
	}

	// Clamp selection
	if dv.selectedIdx >= len(dv.items) {
		dv.selectedIdx = len(dv.items) - 1
	}
	// Skip to first selectable item
	if dv.selectedIdx < 0 {
		dv.selectedIdx = 0
	}
	dv.skipToSelectable(1)
}

func (dv *DetailView) isSelectable(idx int) bool {
	if idx < 0 || idx >= len(dv.items) {
		return false
	}
	return dv.items[idx].kind == "action" || dv.items[idx].kind == "branch"
}

func (dv *DetailView) skipToSelectable(dir int) {
	for dv.selectedIdx >= 0 && dv.selectedIdx < len(dv.items) && !dv.isSelectable(dv.selectedIdx) {
		dv.selectedIdx += dir
	}
	if dv.selectedIdx >= len(dv.items) {
		dv.selectedIdx = len(dv.items) - 1
		for dv.selectedIdx >= 0 && !dv.isSelectable(dv.selectedIdx) {
			dv.selectedIdx--
		}
	}
	if dv.selectedIdx < 0 {
		dv.selectedIdx = 0
	}
}

func (dv *DetailView) moveUp() {
	dv.selectedIdx--
	if dv.selectedIdx < 0 {
		dv.selectedIdx = 0
	}
	// Skip non-selectable items
	for dv.selectedIdx >= 0 && !dv.isSelectable(dv.selectedIdx) {
		dv.selectedIdx--
	}
	if dv.selectedIdx < 0 {
		// Find first selectable
		dv.selectedIdx = 0
		dv.skipToSelectable(1)
	}
}

func (dv *DetailView) moveDown() {
	dv.selectedIdx++
	if dv.selectedIdx >= len(dv.items) {
		dv.selectedIdx = len(dv.items) - 1
	}
	dv.skipToSelectable(1)
}

func (dv *DetailView) handleEnter() tea.Cmd {
	if dv.selectedIdx < 0 || dv.selectedIdx >= len(dv.items) {
		return nil
	}
	item := dv.items[dv.selectedIdx]
	if item.kind != "action" {
		return nil
	}

	switch item.action {
	case "refresh":
		node := dv.node
		return func() tea.Msg {
			scanner.RefreshNode(node)
			return refreshSingleDoneMsg{node: node}
		}
	case "sync":
		return handleSync(dv.node)
	case "code":
		return handleOpenVSCode(dv.node)
	case "explorer":
		return handleOpenExplorer(dv.node)
	case "browser":
		return handleOpenBrowser(dv.node)
	}
	return nil
}

func (dv *DetailView) render(width, height int) string {
	var lines []string
	s := dv.node.Status

	// Repo header
	lines = append(lines, styleHeader.Render("  "+dv.node.Name))
	if s != nil {
		if s.HasRemote {
			lines = append(lines, styleLabel.Render("  ")+styleInfo.Render(s.HTTPSURL))
		}
		lines = append(lines, styleLabel.Render("  Branch: ")+styleValue.Render(s.CurrentBranch))
	}
	lines = append(lines, "")

	// Scrollable list of items
	listHeight := height - 5
	startIdx, endIdx := centeredWindow(dv.selectedIdx, len(dv.items), listHeight)

	for i := startIdx; i < endIdx; i++ {
		item := dv.items[i]
		selected := i == dv.selectedIdx

		switch item.kind {
		case "header":
			lines = append(lines, styleSection.Render("  "+item.label))

		case "separator":
			lines = append(lines, styleTreePrefix.Render("  "+strings.Repeat("─", width-6)))

		case "action":
			cursor := "  "
			if selected {
				cursor = "▸ "
			}
			actionLine := cursor + item.label
			if selected {
				lines = append(lines, styleSelected.Render(actionLine))
			} else {
				lines = append(lines, styleAction.Render(actionLine))
			}

		case "branch":
			b := item.branch
			cursor := "  "
			if selected {
				cursor = "▸ "
			}
			marker := "  "
			if b.IsCurrent {
				marker = "* "
			}
			branchName := fmt.Sprintf("%-18s", b.Name)
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

			branchLine := cursor + marker + branchStyle.Render(branchName) + " " + styleLabel.Render(status)
			if selected {
				lines = append(lines, styleSelected.Render(branchLine))
			} else {
				lines = append(lines, branchLine)
			}
		}
	}

	// Footer hint
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Enter: execute  Esc: back to tree"))

	return strings.Join(lines, "\n")
}
