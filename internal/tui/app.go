package tui

import (
	"fmt"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// ViewMode represents the current view mode.
type ViewMode int

const (
	ViewNormal  ViewMode = iota
	ViewCompare          // Reference file comparison
	ViewRefMenu          // Reference file menu overlay
	ViewHelp             // Help overlay
)

// Model is the main bubbletea model.
type Model struct {
	root        *scanner.TreeNode
	flatNodes   []*scanner.TreeNode
	selectedIdx int

	width  int
	height int

	viewMode    ViewMode
	compareView *CompareView

	statusText  string
	statusError bool

	rootPath   string
	maxDepth   int

	loading    bool
	progress   scanner.ScanProgress
	progressCh chan scanner.ScanProgress
}

// NewModel creates a new TUI model.
func NewModel(rootPath string, maxDepth int) Model {
	return Model{
		rootPath:   rootPath,
		maxDepth:   maxDepth,
		loading:    true,
		progressCh: make(chan scanner.ScanProgress, 100),
	}
}

// scanDoneMsg is sent when initial scan completes.
type scanDoneMsg struct {
	root *scanner.TreeNode
}

// scanProgressMsg carries live progress updates from the scanner.
type scanProgressMsg struct {
	progress scanner.ScanProgress
}

// waitForProgress returns a Cmd that waits for the next progress message on the channel.
func waitForProgress(ch chan scanner.ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return scanProgressMsg{progress: p}
	}
}

// Init initializes the model.
func (m Model) Init() tea.Cmd {
	rootPath := m.rootPath
	maxDepth := m.maxDepth
	ch := m.progressCh

	// Generate default .psmignore if it doesn't exist
	_ = scanner.GenerateDefaultIgnoreFile(rootPath)

	scanCmd := func() tea.Msg {
		root := scanner.ScanDirectory(rootPath, maxDepth, func(p scanner.ScanProgress) {
			// Non-blocking send — drop if the channel buffer is full
			select {
			case ch <- p:
			default:
			}
		})
		close(ch)
		return scanDoneMsg{root: root}
	}

	return tea.Batch(scanCmd, waitForProgress(ch))
}

// Update handles messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case scanProgressMsg:
		m.progress = msg.progress
		// Keep listening for more progress
		return m, waitForProgress(m.progressCh)

	case scanDoneMsg:
		m.root = msg.root
		m.loading = false
		m.rebuildFlatList()
		return m, nil

	case refreshDoneMsg:
		m.root = msg.root
		m.rebuildFlatList()
		m.statusText = "Refreshed"
		m.statusError = false
		m.loading = false
		return m, nil

	case syncDoneMsg:
		if msg.result.Err != nil {
			m.statusText = fmt.Sprintf("Sync failed: %v", msg.result.Err)
			m.statusError = true
		} else {
			m.statusText = msg.result.Message
			m.statusError = false
		}
		// Auto-refresh the affected node
		scanner.RefreshNode(msg.node)
		m.rebuildFlatList()
		return m, nil

	case statusMsg:
		m.statusText = msg.text
		m.statusError = msg.isError
		return m, nil

	case cloneDoneMsg:
		if msg.err != nil {
			m.statusText = fmt.Sprintf("Clone failed for %s: %v", msg.path, msg.err)
			m.statusError = true
		} else {
			m.statusText = fmt.Sprintf("Cloned %s", msg.path)
			m.statusError = false
		}
		return m, nil

	case cloneAllDoneMsg:
		m.statusText = fmt.Sprintf("Cloned %d repos, %d failed", msg.cloned, msg.failed)
		m.statusError = msg.failed > 0
		return m, nil

	case tea.KeyMsg:
		return m.handleKey(msg)
	}

	return m, nil
}

func (m Model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Global keys
	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "?":
		if m.viewMode == ViewHelp {
			m.viewMode = ViewNormal
		} else {
			m.viewMode = ViewHelp
		}
		return m, nil
	}

	// Help overlay - any key dismisses
	if m.viewMode == ViewHelp {
		m.viewMode = ViewNormal
		return m, nil
	}

	// Reference menu
	if m.viewMode == ViewRefMenu {
		return m.handleRefMenuKey(msg)
	}

	// Compare mode keys
	if m.viewMode == ViewCompare && m.compareView != nil {
		return m.handleCompareKey(msg)
	}

	// Normal mode keys
	switch msg.String() {
	case "up", "k":
		if m.selectedIdx > 0 {
			m.selectedIdx--
		}
	case "down", "j":
		if m.selectedIdx < len(m.flatNodes)-1 {
			m.selectedIdx++
		}
	case "right", "l":
		if m.selectedIdx < len(m.flatNodes) {
			node := m.flatNodes[m.selectedIdx]
			if !node.IsGitRepo && len(node.Children) > 0 && !node.Expanded {
				node.Expanded = true
				m.rebuildFlatList()
			}
		}
	case "left", "h":
		if m.selectedIdx < len(m.flatNodes) {
			node := m.flatNodes[m.selectedIdx]
			if node.Expanded && len(node.Children) > 0 {
				node.Expanded = false
				m.rebuildFlatList()
			} else if node.Parent != nil {
				// Navigate to parent
				for i, n := range m.flatNodes {
					if n == node.Parent {
						m.selectedIdx = i
						break
					}
				}
			}
		}
	case "r":
		m.loading = true
		m.statusText = "Refreshing..."
		return m, handleRefresh(m.root)
	case "f":
		m.viewMode = ViewRefMenu
		return m, nil
	case "s":
		return m.handleSyncKey()
	case "c":
		return m.handleActionKey(handleOpenVSCode)
	case "e":
		return m.handleActionKey(handleOpenExplorer)
	case "b":
		return m.handleBrowserKey()
	}

	return m, nil
}

func (m Model) handleSyncKey() (tea.Model, tea.Cmd) {
	if m.selectedIdx >= len(m.flatNodes) {
		return m, nil
	}
	node := m.flatNodes[m.selectedIdx]
	if !node.IsGitRepo || node.Status == nil || !node.Status.HasRemote {
		m.statusText = "Cannot sync: not a git repo with remote"
		m.statusError = true
		return m, nil
	}
	m.statusText = "Syncing..."
	return m, handleSync(node)
}

func (m Model) handleActionKey(fn func(*scanner.TreeNode) tea.Cmd) (tea.Model, tea.Cmd) {
	if m.selectedIdx >= len(m.flatNodes) {
		return m, nil
	}
	return m, fn(m.flatNodes[m.selectedIdx])
}

func (m Model) handleBrowserKey() (tea.Model, tea.Cmd) {
	if m.selectedIdx >= len(m.flatNodes) {
		return m, nil
	}
	node := m.flatNodes[m.selectedIdx]
	if !node.IsGitRepo || node.Status == nil || !node.Status.HasRemote {
		m.statusText = "No remote URL to open"
		m.statusError = true
		return m, nil
	}
	return m, handleOpenBrowser(node)
}

func (m Model) handleRefMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "g":
		// Generate reference file
		ref := reference.Generate(m.root)
		refPath := filepath.Join(m.rootPath, "projects-ref.json")
		if err := reference.Save(ref, refPath); err != nil {
			m.statusText = fmt.Sprintf("Failed to save reference: %v", err)
			m.statusError = true
		} else {
			m.statusText = fmt.Sprintf("Reference saved: %s (%d repos)", refPath, len(ref.Repositories))
			m.statusError = false
		}
		m.viewMode = ViewNormal
		return m, nil
	case "l":
		// Load reference file
		refPath := filepath.Join(m.rootPath, "projects-ref.json")
		ref, err := reference.Load(refPath)
		if err != nil {
			m.statusText = fmt.Sprintf("Failed to load reference: %v", err)
			m.statusError = true
			m.viewMode = ViewNormal
			return m, nil
		}
		result := reference.Compare(ref, m.root)
		m.compareView = newCompareView(result, m.rootPath)
		m.viewMode = ViewCompare
		m.statusText = fmt.Sprintf("Loaded reference: %d matched, %d missing, %d extra",
			len(result.Matched), len(result.Missing), len(result.Extra))
		m.statusError = false
		return m, nil
	case "esc", "q":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

func (m Model) handleCompareKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		if m.compareView.selectedIdx > 0 {
			m.compareView.selectedIdx--
		}
	case "down", "j":
		if m.compareView.selectedIdx < len(m.compareView.entries)-1 {
			m.compareView.selectedIdx++
		}
	case "enter":
		return m, m.compareView.handleClone()
	case "a":
		return m, m.compareView.handleCloneAll()
	case "S":
		return m, m.compareView.handleSyncAll(m.root)
	case "esc":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

func (m *Model) rebuildFlatList() {
	if m.root == nil {
		m.flatNodes = nil
		return
	}
	m.flatNodes = m.root.FlattenVisible()
	if m.selectedIdx >= len(m.flatNodes) {
		m.selectedIdx = len(m.flatNodes) - 1
	}
	if m.selectedIdx < 0 {
		m.selectedIdx = 0
	}
}

// View renders the entire UI.
func (m Model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Initializing..."
	}

	if m.loading && m.root == nil {
		return m.renderLoading()
	}

	// Help overlay
	if m.viewMode == ViewHelp {
		return m.renderHelp()
	}

	// Reference menu overlay
	if m.viewMode == ViewRefMenu {
		return m.renderRefMenu()
	}

	// Calculate panel dimensions
	leftWidth := m.width*2/5 - 2
	rightWidth := m.width - leftWidth - 5
	panelHeight := m.height - 3 // status bar

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel
	var leftContent string
	if m.viewMode == ViewCompare && m.compareView != nil {
		leftContent = m.compareView.renderLeft(leftWidth, panelHeight)
	} else {
		leftContent = renderTree(m.flatNodes, m.selectedIdx, leftWidth, panelHeight)
	}

	leftPanel := stylePanelBorder.
		Width(leftWidth).
		Height(panelHeight).
		Render(leftContent)

	// Right panel
	var rightContent string
	if m.viewMode == ViewCompare && m.compareView != nil {
		rightContent = m.compareView.renderRight(rightWidth, panelHeight)
	} else {
		var selectedNode *scanner.TreeNode
		if m.selectedIdx < len(m.flatNodes) {
			selectedNode = m.flatNodes[m.selectedIdx]
		}
		infoHeight := panelHeight * 2 / 3
		actionHeight := panelHeight - infoHeight - 1
		infoContent := renderInfo(selectedNode, rightWidth, infoHeight)
		actionContent := renderActions(selectedNode, rightWidth)
		rightContent = infoContent + "\n" + strings.Repeat("─", rightWidth-2) + "\n" + actionContent
		_ = actionHeight
	}

	rightPanel := stylePanelBorder.
		Width(rightWidth).
		Height(panelHeight).
		Render(rightContent)

	// Combine panels
	panels := lipgloss.JoinHorizontal(lipgloss.Top, leftPanel, rightPanel)

	// Status bar
	statusStyle := styleSuccess
	if m.statusError {
		statusStyle = styleError
	}
	statusContent := statusStyle.Render(m.statusText)
	commands := styleLabel.Render("  [R]efresh  [F]ile ref  [S]ync  [C]ode  [E]xplorer  [B]rowser  [?]Help  [Q]uit")
	statusBar := styleStatusBar.Width(m.width).Render(statusContent + "  " + commands)

	return panels + "\n" + statusBar
}

func (m Model) renderLoading() string {
	p := m.progress
	var lines []string

	lines = append(lines, styleHeader.Render("  Projects Sync Manager"))
	lines = append(lines, "")

	spinner := [...]string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
	frame := spinner[(p.DirsScanned+p.ReposFetched)%len(spinner)]

	switch p.Phase {
	case "status":
		// Phase 2: fetching git statuses
		lines = append(lines, styleInfo.Render(fmt.Sprintf("  %s Fetching git status...", frame)))
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  Repos: %d / %d", p.ReposFetched, p.ReposTotal)))

		// Progress bar
		if p.ReposTotal > 0 {
			barWidth := 30
			filled := barWidth * p.ReposFetched / p.ReposTotal
			bar := styleGitSynced.Render(strings.Repeat("█", filled)) +
				styleNonGit.Render(strings.Repeat("░", barWidth-filled))
			pct := 100 * p.ReposFetched / p.ReposTotal
			lines = append(lines, fmt.Sprintf("  %s %d%%", bar, pct))
		}

		if p.CurrentDir != "" {
			lines = append(lines, "")
			lines = append(lines, styleLabel.Render("  Current: ")+styleValue.Render(p.CurrentDir))
		}

	default:
		// Phase 1: scanning directories
		lines = append(lines, styleInfo.Render(fmt.Sprintf("  %s Scanning directories...", frame)))
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  Directories scanned: %d", p.DirsScanned)))
		lines = append(lines, styleGitSynced.Render(fmt.Sprintf("  Git repos found:     %d", p.ReposFound)))

		if p.CurrentDir != "" {
			lines = append(lines, "")
			lines = append(lines, styleLabel.Render("  Scanning: ")+styleValue.Render(p.CurrentDir))
		}
	}

	content := strings.Join(lines, "\n")

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(46).
			Padding(1, 2).
			Render(content))
}

func (m Model) renderHelp() string {
	help := `
  Projects Sync Manager - Help

  Navigation
  ──────────
  ↑/↓ or j/k    Navigate between items
  ←/→ or h/l    Collapse/expand directories

  Actions
  ───────
  S              Sync current repo (pull or push)
  C              Open in VS Code
  E              Open in File Explorer
  B              Open remote in Browser
  R              Refresh all statuses

  Reference Files
  ───────────────
  F              Open reference file menu
    G            Generate reference file
    L            Load and compare reference file

  Compare Mode
  ────────────
  Enter          Clone selected missing repo
  A              Clone all missing repos
  Shift+S        Sync all matched repos
  Esc            Exit compare mode

  General
  ───────
  ?              Toggle this help
  Q / Ctrl+C     Quit

  Press any key to dismiss...
`
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(50).
			Padding(1, 2).
			Render(help))
}

func (m Model) renderRefMenu() string {
	menu := `
  Reference File

  [G] Generate reference file
      Save current structure to projects-ref.json

  [L] Load and compare reference file
      Compare projects-ref.json against local state

  [Esc] Cancel
`
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(50).
			Padding(1, 2).
			Render(styleHeader.Render(menu)))
}

// Run starts the TUI application.
func Run(rootPath string, maxDepth int) error {
	p := tea.NewProgram(
		NewModel(rootPath, maxDepth),
		tea.WithAltScreen(),
	)
	_, err := p.Run()
	return err
}
