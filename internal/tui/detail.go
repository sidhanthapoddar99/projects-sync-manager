package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// FocusSection indicates which section of the detail view has focus.
type FocusSection int

const (
	FocusBranches FocusSection = iota
	FocusActions
)

// DetailView manages the interactive repo detail panel.
type DetailView struct {
	node           *scanner.TreeNode
	focus          FocusSection
	branchIdx      int // selected branch index
	actionIdx      int // selected action index
	selectedBranch *git.BranchStatus
	actions        []detailAction
}

type detailAction struct {
	label  string
	action string // key identifier
}

// refreshSingleDoneMsg signals a single repo refresh completed.
type refreshSingleDoneMsg struct {
	node *scanner.TreeNode
}

// syncBranchDoneMsg signals a branch sync completed.
type syncBranchDoneMsg struct {
	result git.SyncResult
	node   *scanner.TreeNode
}

func newDetailView(node *scanner.TreeNode) *DetailView {
	dv := &DetailView{
		node:  node,
		focus: FocusBranches,
	}
	dv.rebuildActions()
	// Auto-select current branch
	if node.Status != nil {
		for i, b := range node.Status.Branches {
			if b.IsCurrent {
				dv.branchIdx = i
				dv.selectBranch(i)
				break
			}
		}
	}
	return dv
}

func (dv *DetailView) selectBranch(idx int) {
	if dv.node.Status == nil || idx < 0 || idx >= len(dv.node.Status.Branches) {
		return
	}
	b := dv.node.Status.Branches[idx]
	dv.selectedBranch = &b
	dv.branchIdx = idx
	dv.rebuildActions()
}

func (dv *DetailView) rebuildActions() {
	dv.actions = nil
	s := dv.node.Status
	if s == nil {
		return
	}

	// Always available
	dv.actions = append(dv.actions, detailAction{"Refresh this repo", "refresh"})

	// Branch-specific sync
	if dv.selectedBranch != nil && s.HasRemote && dv.selectedBranch.IsRemote {
		b := dv.selectedBranch
		syncLabel := fmt.Sprintf("Sync branch: %s", b.Name)
		switch {
		case b.Ahead > 0 && b.Behind > 0:
			syncLabel += " — blocked: diverged"
		case b.Ahead > 0:
			syncLabel += fmt.Sprintf(" — push %d commit(s)", b.Ahead)
		case b.Behind > 0:
			syncLabel += fmt.Sprintf(" — pull %d commit(s)", b.Behind)
		default:
			syncLabel += " — up to date"
		}
		dv.actions = append(dv.actions, detailAction{syncLabel, "sync-branch"})
	}

	// Repo-wide sync (current branch)
	if s.HasRemote {
		dv.actions = append(dv.actions, detailAction{"Sync current branch (S)", "sync-repo"})
	}

	dv.actions = append(dv.actions, detailAction{"Open in VS Code", "code"})
	dv.actions = append(dv.actions, detailAction{"Open in File Explorer", "explorer"})
	if s.HasRemote {
		dv.actions = append(dv.actions, detailAction{"Open in Browser", "browser"})
	}

	// Clamp action index
	if dv.actionIdx >= len(dv.actions) {
		dv.actionIdx = len(dv.actions) - 1
	}
	if dv.actionIdx < 0 {
		dv.actionIdx = 0
	}
}

func (dv *DetailView) rebuild() {
	savedBranch := ""
	if dv.selectedBranch != nil {
		savedBranch = dv.selectedBranch.Name
	}
	dv.rebuildActions()

	// Re-select the same branch after rebuild
	if dv.node.Status != nil && savedBranch != "" {
		for i, b := range dv.node.Status.Branches {
			if b.Name == savedBranch {
				dv.branchIdx = i
				dv.selectBranch(i)
				return
			}
		}
	}
}

func (dv *DetailView) handleKey(msg tea.KeyMsg) tea.Cmd {
	switch msg.String() {
	case "tab":
		if dv.focus == FocusBranches {
			dv.focus = FocusActions
		} else {
			dv.focus = FocusBranches
		}
		return nil

	case "up", "k":
		if dv.focus == FocusBranches {
			if dv.branchIdx > 0 {
				dv.branchIdx--
				dv.selectBranch(dv.branchIdx)
			}
		} else {
			if dv.actionIdx > 0 {
				dv.actionIdx--
			}
		}
		return nil

	case "down", "j":
		if dv.focus == FocusBranches {
			if dv.node.Status != nil && dv.branchIdx < len(dv.node.Status.Branches)-1 {
				dv.branchIdx++
				dv.selectBranch(dv.branchIdx)
			}
		} else {
			if dv.actionIdx < len(dv.actions)-1 {
				dv.actionIdx++
			}
		}
		return nil

	case "enter":
		if dv.focus == FocusBranches {
			// Selecting a branch switches focus to actions for that branch
			dv.selectBranch(dv.branchIdx)
			dv.focus = FocusActions
			dv.actionIdx = 0
			return nil
		}
		// Execute the selected action
		return dv.executeAction()
	}
	return nil
}

func (dv *DetailView) executeAction() tea.Cmd {
	if dv.actionIdx < 0 || dv.actionIdx >= len(dv.actions) {
		return nil
	}
	action := dv.actions[dv.actionIdx]

	switch action.action {
	case "refresh":
		node := dv.node
		return func() tea.Msg {
			scanner.RefreshNode(node)
			return refreshSingleDoneMsg{node: node}
		}
	case "sync-branch":
		if dv.selectedBranch == nil {
			return nil
		}
		node := dv.node
		branch := *dv.selectedBranch
		return func() tea.Msg {
			result := git.SyncBranch(node.Path, branch)
			return syncBranchDoneMsg{result: result, node: node}
		}
	case "sync-repo":
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
	s := dv.node.Status
	if s == nil {
		return styleLabel.Render("  Loading...")
	}

	var lines []string

	// ─── Repo header ───
	lines = append(lines, styleHeader.Render("  "+dv.node.Name))
	if s.HasRemote {
		lines = append(lines, styleLabel.Render("  ")+styleInfo.Render(s.HTTPSURL))
	}
	lines = append(lines, styleLabel.Render("  Current: ")+styleValue.Render(s.CurrentBranch))
	lines = append(lines, "")

	// ─── Working tree summary (one line) ───
	if s.Staged == 0 && s.Unstaged == 0 && s.Untracked == 0 {
		lines = append(lines, styleGitSynced.Render("  Working tree: clean"))
	} else {
		parts := []string{"  Working tree:"}
		if s.Staged > 0 {
			parts = append(parts, styleGitSynced.Render(fmt.Sprintf("+%d staged", s.Staged)))
		}
		if s.Unstaged > 0 {
			parts = append(parts, styleGitDirty.Render(fmt.Sprintf("~%d unstaged", s.Unstaged)))
		}
		if s.Untracked > 0 {
			parts = append(parts, styleNonGit.Render(fmt.Sprintf("…%d untracked", s.Untracked)))
		}
		lines = append(lines, strings.Join(parts, " "))
	}
	lines = append(lines, "")

	// ─── Branch list (upper section) ───
	branchHeader := "  Branches"
	if dv.focus == FocusBranches {
		branchHeader = styleSection.Render(branchHeader + " ◄")
	} else {
		branchHeader = styleLabel.Render(branchHeader)
	}
	lines = append(lines, branchHeader)

	branchAreaHeight := (height - 14) / 2
	if branchAreaHeight < 3 {
		branchAreaHeight = 3
	}

	if len(s.Branches) > 0 {
		startIdx, endIdx := centeredWindow(dv.branchIdx, len(s.Branches), branchAreaHeight)
		for i := startIdx; i < endIdx; i++ {
			b := s.Branches[i]
			isSelected := dv.focus == FocusBranches && i == dv.branchIdx
			isBranchTarget := dv.selectedBranch != nil && b.Name == dv.selectedBranch.Name

			cursor := "  "
			if isSelected {
				cursor = "▸ "
			}
			marker := "  "
			if b.IsCurrent {
				marker = "* "
			}

			branchName := fmt.Sprintf("%-16s", b.Name)
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

			line := cursor + marker + branchStyle.Render(branchName) + " " + styleLabel.Render(status)
			if isBranchTarget && !isSelected {
				line += styleInfo.Render(" ●")
			}
			if isSelected {
				lines = append(lines, styleSelected.Render(line))
			} else {
				lines = append(lines, line)
			}
		}
		if endIdx < len(s.Branches) {
			lines = append(lines, styleTreePrefix.Render(fmt.Sprintf("    ↓ %d more", len(s.Branches)-endIdx)))
		}
	} else {
		lines = append(lines, styleNonGit.Render("    No branches found"))
	}

	// ─── Separator ───
	lines = append(lines, "")
	lines = append(lines, styleTreePrefix.Render("  "+strings.Repeat("─", width-6)))
	lines = append(lines, "")

	// ─── Actions (lower section) ───
	actionHeader := "  Actions"
	if dv.selectedBranch != nil {
		actionHeader += " for " + dv.selectedBranch.Name
	}
	if dv.focus == FocusActions {
		actionHeader = styleSection.Render(actionHeader + " ◄")
	} else {
		actionHeader = styleLabel.Render(actionHeader)
	}
	lines = append(lines, actionHeader)

	for i, a := range dv.actions {
		isSelected := dv.focus == FocusActions && i == dv.actionIdx
		cursor := "  "
		if isSelected {
			cursor = "▸ "
		}
		line := cursor + a.label
		if isSelected {
			lines = append(lines, styleSelected.Render(line))
		} else {
			lines = append(lines, styleAction.Render(line))
		}
	}

	// ─── Footer ───
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Tab: switch section  Enter: select/execute  Esc: back"))

	return strings.Join(lines, "\n")
}
