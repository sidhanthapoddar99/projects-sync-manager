package tui

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/opener"
	"github.com/sid/psm/internal/scanner"
)

// Action messages
type syncDoneMsg struct {
	result git.SyncResult
	node   *scanner.TreeNode
}

type refreshDoneMsg struct {
	root *scanner.TreeNode
}

type statusMsg struct {
	text    string
	isError bool
}

// handleSync initiates a sync operation for the selected node.
func handleSync(node *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		result := git.SyncRepo(node.Path)
		return syncDoneMsg{result: result, node: node}
	}
}

// handleRefresh refreshes all repo statuses.
func handleRefresh(root *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		scanner.RefreshAll(root)
		return refreshDoneMsg{root: root}
	}
}

// handleOpenVSCode opens the selected directory in VS Code.
func handleOpenVSCode(node *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		err := opener.OpenInVSCode(node.Path)
		if err != nil {
			return statusMsg{text: fmt.Sprintf("Failed to open VS Code: %v", err), isError: true}
		}
		return statusMsg{text: "Opened in VS Code", isError: false}
	}
}

// handleOpenExplorer opens the selected directory in file explorer.
func handleOpenExplorer(node *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		err := opener.OpenInExplorer(node.Path)
		if err != nil {
			return statusMsg{text: fmt.Sprintf("Failed to open explorer: %v", err), isError: true}
		}
		return statusMsg{text: "Opened in file explorer", isError: false}
	}
}

// handleOpenBrowser opens the remote repo URL in browser.
func handleOpenBrowser(node *scanner.TreeNode) tea.Cmd {
	return func() tea.Msg {
		if node.Status == nil || !node.Status.HasRemote {
			return statusMsg{text: "No remote URL available", isError: true}
		}
		err := opener.OpenInBrowser(node.Status.HTTPSURL)
		if err != nil {
			return statusMsg{text: fmt.Sprintf("Failed to open browser: %v", err), isError: true}
		}
		return statusMsg{text: "Opened in browser", isError: false}
	}
}
