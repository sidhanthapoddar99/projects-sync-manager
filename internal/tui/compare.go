package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// CompareView manages the reference file comparison view.
type CompareView struct {
	result      *reference.CompareResult
	entries     []compareEntry
	selectedIdx int
	rootPath    string
}

type compareEntry struct {
	indicator string // [==], [++], [--]
	path      string
	remoteURL string
	status    string // "matched", "missing", "extra"
	node      *scanner.TreeNode
}

type cloneDoneMsg struct {
	path string
	err  error
}

type cloneAllDoneMsg struct {
	cloned int
	failed int
}

func newCompareView(result *reference.CompareResult, rootPath string) *CompareView {
	cv := &CompareView{
		result:   result,
		rootPath: rootPath,
	}

	for _, e := range result.Matched {
		cv.entries = append(cv.entries, compareEntry{
			indicator: "[==]",
			path:      e.RelativePath,
			remoteURL: e.RemoteURL,
			status:    "matched",
			node:      e.LocalNode,
		})
	}
	for _, e := range result.Missing {
		cv.entries = append(cv.entries, compareEntry{
			indicator: "[--]",
			path:      e.RelativePath,
			remoteURL: e.RemoteURL,
			status:    "missing",
		})
	}
	for _, e := range result.Extra {
		cv.entries = append(cv.entries, compareEntry{
			indicator: "[++]",
			path:      e.RelativePath,
			remoteURL: e.RemoteURL,
			status:    "extra",
			node:      e.LocalNode,
		})
	}

	return cv
}

func (cv *CompareView) renderLeft(width, height int) string {
	var lines []string
	lines = append(lines, styleHeader.Render("  Reference Comparison"))
	lines = append(lines, "")

	summary := fmt.Sprintf("  Matched: %d  Missing: %d  Extra: %d",
		len(cv.result.Matched), len(cv.result.Missing), len(cv.result.Extra))
	lines = append(lines, styleLabel.Render(summary))
	lines = append(lines, "")

	listHeight := height - 6
	startIdx, endIdx := centeredWindow(cv.selectedIdx, len(cv.entries), listHeight)

	for i := startIdx; i < endIdx; i++ {
		e := cv.entries[i]
		var styledIndicator string
		switch e.status {
		case "matched":
			styledIndicator = styleMatchedIndicator.Render(e.indicator)
		case "missing":
			styledIndicator = styleMissingIndicator.Render(e.indicator)
		case "extra":
			styledIndicator = styleExtraIndicator.Render(e.indicator)
		}

		line := "  " + styledIndicator + " " + e.path
		if i == cv.selectedIdx {
			line = styleSelected.Render(line)
		}
		lines = append(lines, line)
	}

	return strings.Join(lines, "\n")
}

func (cv *CompareView) renderRight(width, height int) string {
	if len(cv.entries) == 0 {
		return styleLabel.Render("  No entries to display")
	}

	e := cv.entries[cv.selectedIdx]
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

func (cv *CompareView) handleClone() tea.Cmd {
	if cv.selectedIdx >= len(cv.entries) {
		return nil
	}
	e := cv.entries[cv.selectedIdx]
	if e.status != "missing" {
		return nil
	}
	targetPath := filepath.Join(cv.rootPath, e.path)
	return func() tea.Msg {
		err := git.CloneRepo(e.remoteURL, targetPath)
		return cloneDoneMsg{path: e.path, err: err}
	}
}

func (cv *CompareView) handleCloneAll() tea.Cmd {
	var missing []compareEntry
	for _, e := range cv.entries {
		if e.status == "missing" {
			missing = append(missing, e)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	rootPath := cv.rootPath
	return func() tea.Msg {
		cloned, failed := 0, 0
		for _, e := range missing {
			targetPath := filepath.Join(rootPath, e.path)
			err := git.CloneRepo(e.remoteURL, targetPath)
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
		for _, e := range cv.entries {
			if e.status == "matched" && e.node != nil && e.node.Status != nil {
				s := e.node.Status
				if s.Staged == 0 && s.Unstaged == 0 {
					git.SyncRepo(e.node.Path)
				}
			}
		}
		scanner.RefreshAll(root)
		return refreshDoneMsg{root: root}
	}
}
