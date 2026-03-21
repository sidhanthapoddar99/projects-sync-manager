package tui

import (
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/network"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// busyProgressMsg carries progress updates during busy operations.
type busyProgressMsg struct {
	done    int
	total   int
	current string
}

// pendingRenameInfo stores details for a folder rename confirmation.
type pendingRenameInfo struct {
	node    *scanner.TreeNode
	oldPath string
	newPath string
	newName string
}

// renameDoneMsg is sent after a folder rename completes.
type renameDoneMsg struct {
	node    *scanner.TreeNode
	oldPath string
	newPath string
	err     error
}

// pendingCloneInfo stores clone details while waiting for SSH/HTTPS choice.
type pendingCloneInfo struct {
	relPath   string // relative path for single clone ("" if cloneAll)
	remoteURL string // HTTPS URL for single clone
	cloneAll  bool   // true if this is a "clone all" operation
	host      string // extracted host for display
}

// sshCheckDoneMsg is sent after SSH access test completes.
type sshCheckDoneMsg struct {
	available bool
	host      string
}

// ViewMode represents the current view mode.
type ViewMode int

const (
	ViewNormal            ViewMode = iota
	ViewCompare                    // Reference file comparison
	ViewRefMenu                    // Reference file menu overlay
	ViewHelp                       // Help overlay
	ViewDetail                     // Repo detail / interactive right panel
	ViewClonePrompt                // SSH/HTTPS clone choice overlay
	ViewFilter                     // Filter panel overlay
	ViewCommand                    // Command palette overlay
	ViewRenameConfirm              // Rename folder confirmation
	ViewBusy                       // Blocking progress overlay
	ViewNetworkMenu                // Network: Start Server / Connect
	ViewNetworkServerWait          // Network: server running, waiting for client
	ViewNetworkClientInput         // Network: URL + code input
	ViewPeerCompare                // Network: live peer comparison
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

	detailView *DetailView

	confirmRefreshAll bool // waiting for y/n confirmation on full refresh

	// Filters
	filters        FilterSet
	filterPanelIdx int // cursor position in filter panel

	// Command palette
	commandPalette *CommandPalette

	// Rename confirmation
	pendingRename *pendingRenameInfo

	// Busy overlay
	busyTitle    string // e.g. "Refreshing All Repos" or "Syncing"
	busyDone     int
	busyTotal    int
	busyCurrent  string // name of repo currently being processed
	busyCh       chan scanner.ScanProgress

	// SSH clone preference (session-wide)
	cloneSSHAll  bool  // user chose "A" — SSH for all clones this session
	sshChecked   bool  // have we tested SSH access?
	sshAvailable bool  // result of SSH access test
	pendingClone *pendingCloneInfo // clone waiting for user's SSH/HTTPS choice

	// Network peer sync
	peer            *network.Peer
	peerTree        *network.PeerTree
	peerCompareTab  int              // 0=combined, 1=local, 2=remote, 3=my tree, 4=peer tree
	httpServer      *http.Server
	serverCode      string
	serverIPs       []string
	networkInput    [3]string        // [0]=port, [1]=url, [2]=code
	networkField    int              // active input field (client form)
	peerRoot        *scanner.TreeNode   // virtual tree from peer's data (tab 4)
	peerFlatNodes   []*scanner.TreeNode // flattened peer tree (tab 4)
	peerSelectedIdx int                 // cursor in peer tree (tab 4)
}

// NewModel creates a new TUI model.
func NewModel(rootPath string, maxDepth int) Model {
	return Model{
		rootPath:   rootPath,
		maxDepth:   maxDepth,
		loading:    true,
		progressCh: make(chan scanner.ScanProgress, 100),
		filters:    NewFilterSet(),
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

// waitForBusyProgress blocks on the busy channel and returns progress messages.
func waitForBusyProgress(ch chan scanner.ScanProgress) tea.Cmd {
	return func() tea.Msg {
		p, ok := <-ch
		if !ok {
			return nil
		}
		return busyProgressMsg{done: p.ReposFetched, total: p.ReposTotal, current: p.CurrentDir}
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

	case busyProgressMsg:
		m.busyDone = msg.done
		m.busyTotal = msg.total
		m.busyCurrent = msg.current
		if m.busyCh != nil {
			return m, waitForBusyProgress(m.busyCh)
		}
		return m, nil

	case refreshDoneMsg:
		m.root = msg.root
		m.rebuildFlatList()
		m.statusText = "Refreshed"
		m.statusError = false
		m.loading = false
		if m.viewMode != ViewPeerCompare {
			m.viewMode = ViewNormal
		}
		m.busyCh = nil
		m.sendTreeToPeer()
		return m, nil

	case refreshSingleDoneMsg:
		if m.detailView != nil && m.detailView.node == msg.node {
			m.detailView.rebuild()
		}
		m.rebuildFlatList()
		m.statusText = fmt.Sprintf("Refreshed %s", msg.node.Name)
		m.statusError = false
		if m.viewMode == ViewBusy {
			m.viewMode = ViewNormal
			m.busyCh = nil
		}
		m.sendTreeToPeer()
		return m, nil

	case syncBranchDoneMsg:
		if msg.result.Err != nil {
			m.statusText = fmt.Sprintf("Branch sync failed: %v", msg.result.Err)
			m.statusError = true
		} else {
			m.statusText = msg.result.Message
			m.statusError = false
		}
		scanner.RefreshNode(msg.node)
		if m.detailView != nil && m.detailView.node == msg.node {
			m.detailView.rebuild()
		}
		m.rebuildFlatList()
		m.sendTreeToPeer()
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
		if m.detailView != nil && m.detailView.node == msg.node {
			m.detailView.rebuild()
		}
		m.rebuildFlatList()
		if m.viewMode == ViewBusy {
			m.viewMode = ViewNormal
			m.busyCh = nil
		}
		m.sendTreeToPeer()
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
			clonedPath := filepath.Join(m.rootPath, msg.path)
			// Insert into the scanner tree and get status
			newNode := scanner.InsertNode(m.root, clonedPath)
			if newNode != nil {
				m.rebuildFlatList()
			}
			// Update compare view if active
			if (m.viewMode == ViewCompare || m.viewMode == ViewPeerCompare) && m.compareView != nil {
				m.compareView.markCloned(msg.path, newNode)
			}
			m.sendTreeToPeer()
		}
		return m, nil

	case cloneAllDoneMsg:
		m.statusText = fmt.Sprintf("Cloned %d repos, %d failed", msg.cloned, msg.failed)
		m.statusError = msg.failed > 0
		if m.viewMode == ViewCompare && m.compareView != nil {
			// Re-run the comparison with updated scanner tree
			ref, err := reference.Load(filepath.Join(m.rootPath, "projects-ref.json"))
			if err == nil {
				result := reference.Compare(ref, m.root)
				m.compareView = newCompareView(result, m.rootPath)
			}
		}
		if m.viewMode == ViewPeerCompare {
			m.rebuildPeerCompareView()
		}
		m.sendTreeToPeer()
		return m, nil

	case renameRequestMsg:
		node := msg.node
		if node.Status != nil && node.Status.NameMismatch() {
			newName := node.Status.RepoName
			oldPath := node.Path
			newPath := filepath.Join(filepath.Dir(oldPath), newName)
			m.pendingRename = &pendingRenameInfo{
				node:    node,
				oldPath: oldPath,
				newPath: newPath,
				newName: newName,
			}
			m.viewMode = ViewRenameConfirm
		}
		return m, nil

	case renameDoneMsg:
		if msg.err != nil {
			m.statusText = fmt.Sprintf("Rename failed: %v", msg.err)
			m.statusError = true
		} else {
			// Update the node's path and name in the tree
			msg.node.Path = msg.newPath
			msg.node.Name = filepath.Base(msg.newPath)
			// Refresh status since path changed
			scanner.RefreshNode(msg.node)
			m.rebuildFlatList()
			m.statusText = fmt.Sprintf("Renamed to %s", filepath.Base(msg.newPath))
			m.statusError = false
		}
		return m, nil

	case peerConnectedMsg:
		m.peer = msg.peer
		m.viewMode = ViewPeerCompare
		m.peerCompareTab = 0
		m.statusText = fmt.Sprintf("Connected to %s", msg.hostname)
		m.statusError = false
		// Send our tree to the peer
		m.sendTreeToPeer()
		// Start listening for peer messages
		return m, waitForPeerMsg(m.peer.InCh)

	case peerMessageMsg:
		cmd := m.handlePeerMessage(msg.msg)
		// Keep listening
		if m.peer != nil {
			return m, tea.Batch(cmd, waitForPeerMsg(m.peer.InCh))
		}
		return m, cmd

	case peerDisconnectedMsg:
		m.peer = nil
		m.peerTree = nil
		if m.httpServer != nil {
			network.ShutdownServer(m.httpServer)
			m.httpServer = nil
		}
		m.serverCode = ""
		if m.viewMode == ViewPeerCompare || m.viewMode == ViewNetworkServerWait {
			m.viewMode = ViewNormal
		}
		m.statusText = "Peer disconnected"
		m.statusError = false
		return m, nil

	case peerActionDoneMsg:
		// A locally-executed action triggered by peer request completed
		if msg.err != nil {
			m.statusText = fmt.Sprintf("Peer-requested %s failed: %v", msg.action, msg.err)
			m.statusError = true
			if m.peer != nil {
				go m.peer.Send(network.MsgActionResult, network.ActionResultData{
					Action: msg.action, Path: msg.path, Error: msg.err.Error(),
				})
			}
		} else {
			m.statusText = fmt.Sprintf("Peer-requested %s: %s", msg.action, msg.message)
			m.statusError = false
			if msg.action == "clone" {
				clonedPath := filepath.Join(m.rootPath, msg.path)
				scanner.InsertNode(m.root, clonedPath)
				m.rebuildFlatList()
			} else if msg.action == "sync" {
				// Refresh the synced node
				node := findNodeByPath(m.root, filepath.Join(m.rootPath, msg.path))
				if node != nil {
					scanner.RefreshNode(node)
					m.rebuildFlatList()
				}
			}
			if m.peer != nil {
				go m.peer.Send(network.MsgActionResult, network.ActionResultData{
					Action: msg.action, Path: msg.path, Message: msg.message,
				})
			}
			m.sendTreeToPeer()
		}
		return m, nil

	case connectFailedMsg:
		m.statusText = fmt.Sprintf("Connection failed: %v", msg.err)
		m.statusError = true
		m.viewMode = ViewNormal
		return m, nil

	case sshCheckDoneMsg:
		m.sshChecked = true
		m.sshAvailable = msg.available
		if m.pendingClone == nil {
			return m, nil
		}
		if !msg.available {
			// SSH not available, clone with HTTPS directly
			m.statusText = "SSH not available, cloning via HTTPS..."
			return m, m.executePendingClone(false)
		}
		// SSH available — show the prompt
		m.viewMode = ViewClonePrompt
		m.statusText = fmt.Sprintf("SSH access to %s available", msg.host)
		m.statusError = false
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
		m.disconnectPeer()
		return m, tea.Quit
	case "?":
		if m.viewMode == ViewHelp {
			m.viewMode = ViewNormal
		} else {
			m.viewMode = ViewHelp
		}
		return m, nil
	}

	// Busy overlay - block all input
	if m.viewMode == ViewBusy {
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

	// Clone prompt keys
	if m.viewMode == ViewClonePrompt {
		return m.handleClonePromptKey(msg)
	}

	// Rename confirmation keys
	if m.viewMode == ViewRenameConfirm {
		return m.handleRenameConfirmKey(msg)
	}

	// Filter panel keys
	if m.viewMode == ViewFilter {
		return m.handleFilterKey(msg)
	}

	// Command palette keys
	if m.viewMode == ViewCommand && m.commandPalette != nil {
		return m.handleCommandKey(msg)
	}

	// Network menu keys
	if m.viewMode == ViewNetworkMenu {
		return m.handleNetworkMenuKey(msg)
	}

	// Network server wait keys
	if m.viewMode == ViewNetworkServerWait {
		return m.handleNetworkServerWaitKey(msg)
	}

	// Network client input keys
	if m.viewMode == ViewNetworkClientInput {
		return m.handleNetworkClientInputKey(msg)
	}

	// Peer compare keys
	if m.viewMode == ViewPeerCompare && m.compareView != nil {
		return m.handlePeerCompareKey(msg)
	}

	// Compare mode keys
	if m.viewMode == ViewCompare && m.compareView != nil {
		return m.handleCompareKey(msg)
	}

	// Detail mode keys
	if m.viewMode == ViewDetail && m.detailView != nil {
		return m.handleDetailKey(msg)
	}

	// Confirm refresh all prompt
	if m.confirmRefreshAll {
		switch msg.String() {
		case "y", "Y":
			m.confirmRefreshAll = false
			m.viewMode = ViewBusy
			m.busyTitle = "Refreshing All Repos"
			m.busyDone = 0
			m.busyTotal = 0
			m.busyCurrent = ""
			ch := make(chan scanner.ScanProgress, 100)
			m.busyCh = ch
			root := m.root
			refreshCmd := func() tea.Msg {
				scanner.RefreshAllWithProgress(root, func(p scanner.ScanProgress) {
					select {
					case ch <- p:
					default:
					}
				})
				close(ch)
				return refreshDoneMsg{root: root}
			}
			return m, tea.Batch(refreshCmd, waitForBusyProgress(ch))
		default:
			m.confirmRefreshAll = false
			m.statusText = "Refresh all cancelled"
			m.statusError = false
			return m, nil
		}
	}

	// Normal mode keys
	switch msg.String() {
	case "up", "k":
		m.selectedIdx = navigateUp(m.flatNodes, m.selectedIdx, m.filters.IsActive())
	case "down", "j":
		m.selectedIdx = navigateDown(m.flatNodes, m.selectedIdx, m.filters.IsActive())
	case "right", "l":
		if m.selectedIdx < len(m.flatNodes) {
			targetNode := m.flatNodes[m.selectedIdx] // save reference before rebuild
			newIdx, changed := navigateRight(m.flatNodes, m.selectedIdx)
			if changed {
				m.rebuildFlatList()
				// Find the expanded node in the new flat list and select its first child
				for i, n := range m.flatNodes {
					if n == targetNode {
						if len(targetNode.Children) > 0 {
							// Find first child in new flat list
							for j, c := range m.flatNodes {
								if c == targetNode.Children[0] {
									m.selectedIdx = j
									break
								}
							}
						} else {
							m.selectedIdx = i
						}
						break
					}
				}
			} else {
				m.selectedIdx = newIdx
			}
		}
	case "left", "h":
		newIdx, changed := navigateLeft(m.flatNodes, m.selectedIdx)
		if changed {
			m.rebuildFlatList()
		}
		m.selectedIdx = newIdx
	case "enter":
		if m.selectedIdx < len(m.flatNodes) {
			node := m.flatNodes[m.selectedIdx]
			if node.IsGitRepo && node.Status != nil {
				m.detailView = newDetailView(node)
				m.viewMode = ViewDetail
				return m, nil
			}
		}
	case "r":
		// Individual refresh — just the selected repo
		if m.selectedIdx < len(m.flatNodes) {
			node := m.flatNodes[m.selectedIdx]
			if node.IsGitRepo {
				m.viewMode = ViewBusy
				m.busyTitle = "Refreshing"
				m.busyDone = 0
				m.busyTotal = 1
				m.busyCurrent = node.Name
				m.busyCh = nil
				return m, func() tea.Msg {
					scanner.RefreshNode(node)
					return refreshSingleDoneMsg{node: node}
				}
			}
		}
	case "R":
		// Full refresh with confirmation
		m.confirmRefreshAll = true
		m.statusText = "Refresh ALL repos from remote? This may take a while. (y/n)"
		m.statusError = false
		return m, nil
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
	case "n":
		// Rename folder to match repo name
		if m.selectedIdx < len(m.flatNodes) {
			node := m.flatNodes[m.selectedIdx]
			if node.IsGitRepo && node.Status != nil && node.Status.NameMismatch() {
				newName := node.Status.RepoName
				oldPath := node.Path
				newPath := filepath.Join(filepath.Dir(oldPath), newName)
				m.pendingRename = &pendingRenameInfo{
					node:    node,
					oldPath: oldPath,
					newPath: newPath,
					newName: newName,
				}
				m.viewMode = ViewRenameConfirm
				return m, nil
			}
			m.statusText = "No name mismatch to fix"
			m.statusError = true
			return m, nil
		}
	case "N":
		m.viewMode = ViewNetworkMenu
		return m, nil
	case "F":
		m.viewMode = ViewFilter
		return m, nil
	case "/":
		m.commandPalette = newCommandPalette(&m)
		m.viewMode = ViewCommand
		return m, nil
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
	m.viewMode = ViewBusy
	m.busyTitle = "Syncing"
	m.busyDone = 0
	m.busyTotal = 1
	m.busyCurrent = node.Name
	m.busyCh = nil
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
		statusParts := fmt.Sprintf("Loaded reference: %d matched, %d missing, %d extra",
			len(result.Matched), len(result.Missing), len(result.Extra))
		if len(result.Relocated) > 0 {
			statusParts += fmt.Sprintf(", %d relocated", len(result.Relocated))
		}
		m.statusText = statusParts
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
		m.compareView.navigateUp()
	case "down", "j":
		m.compareView.navigateDown()
	case "right", "l":
		m.compareView.navigateRight()
	case "left", "h":
		m.compareView.navigateLeft()
	case "enter":
		// Clone single — route through SSH check
		e := m.compareView.selectedEntry()
		if e == nil || e.status != "missing" {
			return m, nil
		}
		return m.initiateClone(e.path, e.remoteURL, false)
	case "a":
		// Clone all — route through SSH check
		// Find first missing entry to get the host
		var firstMissing *compareEntry
		m.compareView.walkEntries(func(entry *compareEntry) {
			if entry.status == "missing" && firstMissing == nil {
				firstMissing = entry
			}
		})
		if firstMissing == nil {
			return m, nil
		}
		return m.initiateClone("", firstMissing.remoteURL, true)
	case "S":
		return m, m.compareView.handleSyncAll(m.root)
	case "esc":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

// initiateClone starts the clone process, checking SSH availability first.
func (m Model) initiateClone(relPath, remoteURL string, cloneAll bool) (tea.Model, tea.Cmd) {
	host := git.ExtractHost(remoteURL)
	m.pendingClone = &pendingCloneInfo{
		relPath:   relPath,
		remoteURL: remoteURL,
		cloneAll:  cloneAll,
		host:      host,
	}

	// If user already chose "SSH for all", skip the prompt
	if m.cloneSSHAll {
		m.statusText = "Cloning via SSH..."
		return m, m.executePendingClone(true)
	}

	// If we already checked SSH for this session, skip the async check
	if m.sshChecked {
		if !m.sshAvailable {
			m.statusText = "Cloning via HTTPS..."
			return m, m.executePendingClone(false)
		}
		// SSH available, show prompt
		m.viewMode = ViewClonePrompt
		return m, nil
	}

	// First time — run async SSH check
	m.statusText = fmt.Sprintf("Checking SSH access to %s...", host)
	return m, func() tea.Msg {
		available := git.CheckSSHAccess(host)
		return sshCheckDoneMsg{available: available, host: host}
	}
}

// executePendingClone runs the actual clone with SSH or HTTPS.
func (m Model) executePendingClone(useSSH bool) tea.Cmd {
	pc := m.pendingClone
	if pc == nil {
		return nil
	}

	if pc.cloneAll {
		return m.compareView.handleCloneAllWithProtocol(useSSH, m.rootPath)
	}

	cloneURL := pc.remoteURL
	if useSSH {
		cloneURL = git.HTTPSToSSH(pc.remoteURL)
	}
	relPath := pc.relPath
	rootPath := m.rootPath
	return func() tea.Msg {
		targetPath := filepath.Join(rootPath, relPath)
		err := git.CloneRepo(cloneURL, targetPath)
		return cloneDoneMsg{path: relPath, err: err}
	}
}

func (m Model) handleClonePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Return to whichever compare view was active
	returnView := ViewCompare
	if m.peer != nil && m.peerTree != nil {
		returnView = ViewPeerCompare
	}
	switch msg.String() {
	case "a", "A":
		// SSH for all clones this session
		m.cloneSSHAll = true
		m.viewMode = returnView
		m.statusText = "Cloning via SSH (all future clones will use SSH)..."
		return m, m.executePendingClone(true)
	case "y", "Y":
		// SSH just this one
		m.viewMode = returnView
		m.statusText = "Cloning via SSH..."
		return m, m.executePendingClone(true)
	case "h", "H":
		// HTTPS for this one
		m.viewMode = returnView
		m.statusText = "Cloning via HTTPS..."
		return m, m.executePendingClone(false)
	case "esc":
		// Cancel
		m.viewMode = returnView
		m.pendingClone = nil
		m.statusText = "Clone cancelled"
		return m, nil
	}
	return m, nil
}

func (m Model) handleRenameConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		pr := m.pendingRename
		m.viewMode = ViewNormal
		m.pendingRename = nil
		if pr != nil {
			m.statusText = fmt.Sprintf("Renaming %s → %s...", filepath.Base(pr.oldPath), pr.newName)
			return m, func() tea.Msg {
				err := os.Rename(pr.oldPath, pr.newPath)
				return renameDoneMsg{node: pr.node, oldPath: pr.oldPath, newPath: pr.newPath, err: err}
			}
		}
	default:
		m.viewMode = ViewNormal
		m.pendingRename = nil
		m.statusText = "Rename cancelled"
		m.statusError = false
	}
	return m, nil
}

func (m Model) handleFilterKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	allFilters := AllFilterTypes()
	switch msg.String() {
	case "esc":
		m.viewMode = ViewNormal
		return m, nil
	case "up", "k":
		if m.filterPanelIdx > 0 {
			m.filterPanelIdx--
		}
	case "down", "j":
		if m.filterPanelIdx < len(allFilters)-1 {
			m.filterPanelIdx++
		}
	case " ", "enter":
		if m.filterPanelIdx < len(allFilters) {
			m.filters.Toggle(allFilters[m.filterPanelIdx])
			m.rebuildFlatList()
			if m.filters.IsActive() {
				count := m.filters.CountMatches(m.root.FlattenVisible())
				m.statusText = fmt.Sprintf("Filter: %s (%d repos)", m.filters.ActiveNames(), count)
				m.statusError = false
			} else {
				m.statusText = "Filters cleared"
				m.statusError = false
			}
		}
	case "c":
		m.filters.Clear()
		m.rebuildFlatList()
		m.statusText = "Filters cleared"
		m.statusError = false
	}
	return m, nil
}

func (m Model) handleCommandKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	result := m.commandPalette.handleKey(msg)
	switch result.action {
	case cmdActionClose:
		m.viewMode = ViewNormal
		m.commandPalette = nil
	case cmdActionExecute:
		m.viewMode = ViewNormal
		cmd := result.command
		m.commandPalette = nil
		if cmd != nil {
			return cmd.Execute(&m)
		}
	}
	return m, nil
}

func (m Model) handleDetailKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		// Return to peer compare if connected, else normal
		if m.peer != nil && m.peerTree != nil {
			m.viewMode = ViewPeerCompare
		} else {
			m.viewMode = ViewNormal
		}
		m.detailView = nil
		return m, nil
	default:
		cmd := m.detailView.handleKey(msg)
		if cmd != nil {
			return m, cmd
		}
	}
	return m, nil
}

// findNodeByPath walks the tree and returns the node with the given path, or nil.
func findNodeByPath(root *scanner.TreeNode, path string) *scanner.TreeNode {
	if root.Path == path {
		return root
	}
	for _, c := range root.Children {
		if n := findNodeByPath(c, path); n != nil {
			return n
		}
	}
	return nil
}

func (m *Model) rebuildFlatList() {
	if m.root == nil {
		m.flatNodes = nil
		return
	}
	visible := m.root.FlattenVisible()
	m.flatNodes = m.filters.Apply(visible)
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

	// Busy overlay
	if m.viewMode == ViewBusy {
		return m.renderBusyOverlay()
	}

	// Help overlay
	if m.viewMode == ViewHelp {
		return m.renderHelp()
	}

	// Reference menu overlay
	if m.viewMode == ViewRefMenu {
		return m.renderRefMenu()
	}

	// Filter panel overlay
	if m.viewMode == ViewFilter {
		return renderFilterPanel(&m.filters, m.filterPanelIdx, m.width, m.height)
	}

	// Command palette overlay
	if m.viewMode == ViewCommand && m.commandPalette != nil {
		return m.commandPalette.render(m.width, m.height)
	}

	// Network menu overlay
	if m.viewMode == ViewNetworkMenu {
		return m.renderNetworkMenu()
	}

	// Network server wait overlay
	if m.viewMode == ViewNetworkServerWait {
		return m.renderNetworkServerWait()
	}

	// Network client input overlay
	if m.viewMode == ViewNetworkClientInput {
		return m.renderNetworkClientInput()
	}

	// Rename confirmation overlay
	if m.viewMode == ViewRenameConfirm && m.pendingRename != nil {
		return m.renderRenameConfirm()
	}

	// Clone prompt overlay
	if m.viewMode == ViewClonePrompt {
		return m.renderClonePrompt()
	}

	// Calculate panel dimensions
	leftWidth := m.width*2/5 - 2
	rightWidth := m.width - leftWidth - 5
	panelHeight := m.height - 4 // status bar (2 lines) + borders

	if leftWidth < 20 {
		leftWidth = 20
	}
	if rightWidth < 20 {
		rightWidth = 20
	}

	// Left panel
	var leftContent string
	if m.viewMode == ViewPeerCompare && m.peerCompareTab == 3 {
		// Tab 3: My Tree (normal local view with tab bar)
		tabBar := m.renderPeerTabBar(leftWidth)
		var filterStatus string
		if m.filters.IsActive() {
			count := m.filters.CountMatches(m.root.FlattenVisible())
			filterStatus = fmt.Sprintf("Filter: %s (%d repos)", m.filters.ActiveNames(), count)
		}
		leftContent = tabBar + "\n" + renderTree(m.flatNodes, m.selectedIdx, leftWidth, panelHeight-1, filterStatus)
	} else if m.viewMode == ViewPeerCompare && m.peerCompareTab == 4 {
		// Tab 4: Peer Tree (peer's repos with tab bar)
		tabBar := m.renderPeerTabBar(leftWidth)
		leftContent = tabBar + "\n" + renderTree(m.peerFlatNodes, m.peerSelectedIdx, leftWidth, panelHeight-1, "")
	} else if (m.viewMode == ViewCompare || m.viewMode == ViewPeerCompare) && m.compareView != nil {
		leftContent = m.compareView.renderLeft(leftWidth, panelHeight)
	} else {
		var filterStatus string
		if m.filters.IsActive() {
			count := m.filters.CountMatches(m.root.FlattenVisible())
			filterStatus = fmt.Sprintf("Filter: %s (%d repos)", m.filters.ActiveNames(), count)
		}
		leftContent = renderTree(m.flatNodes, m.selectedIdx, leftWidth, panelHeight, filterStatus)
	}

	leftPanel := stylePanelBorder.
		Width(leftWidth).
		Height(panelHeight).
		Render(leftContent)

	// Right panel
	var rightContent string
	if m.viewMode == ViewDetail && m.detailView != nil {
		rightContent = m.detailView.render(rightWidth, panelHeight)
	} else if m.viewMode == ViewPeerCompare && m.peerCompareTab == 3 {
		// Tab 3: normal info + actions for local tree
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
	} else if m.viewMode == ViewPeerCompare && m.peerCompareTab == 4 {
		// Tab 4: info + remote actions for peer tree
		var selectedNode *scanner.TreeNode
		if m.peerSelectedIdx < len(m.peerFlatNodes) {
			selectedNode = m.peerFlatNodes[m.peerSelectedIdx]
		}
		infoHeight := panelHeight * 2 / 3
		actionHeight := panelHeight - infoHeight - 1
		infoContent := renderInfo(selectedNode, rightWidth, infoHeight)
		actionContent := renderPeerActions(selectedNode, rightWidth)
		rightContent = infoContent + "\n" + strings.Repeat("─", rightWidth-2) + "\n" + actionContent
		_ = actionHeight
	} else if (m.viewMode == ViewCompare || m.viewMode == ViewPeerCompare) && m.compareView != nil {
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
	var navLine string
	if m.viewMode == ViewDetail {
		navLine = styleLabel.Render("  ↑↓") + styleValue.Render(" navigate  ") +
			styleAction.Render("Tab") + styleValue.Render(" switch section  ") +
			styleAction.Render("Enter") + styleValue.Render(" select/execute  ") +
			styleAction.Render("Esc") + styleValue.Render(" back  ") +
			styleLabel.Render("│ ") +
			styleAction.Render("?") + styleLabel.Render("Help ") +
			styleAction.Render("Q") + styleLabel.Render("uit")
	} else if m.viewMode == ViewPeerCompare {
		pName := m.peerName()
		navLine = styleLabel.Render("  ↑↓") + styleValue.Render(" navigate  ") +
			styleAction.Render("1-5") + styleValue.Render(" tabs  ") +
			styleAction.Render("Enter") + styleValue.Render(" action  ") +
			styleAction.Render("D") + styleValue.Render(" disconnect  ") +
			styleAction.Render("Esc") + styleValue.Render(" back  ") +
			styleLabel.Render("│ ") +
			styleGitSynced.Render(fmt.Sprintf("[Peer: %s]", pName))
	} else {
	if m.confirmRefreshAll {
		navLine = styleGitDirty.Render("  Press Y to confirm full refresh, any other key to cancel")
	} else {
		filterBadge := ""
		if m.filters.IsActive() {
			filterBadge = styleGitPartial.Render(fmt.Sprintf("(%d)", m.filters.ActiveCount()))
		}
		peerBadge := ""
		if m.peer != nil {
			peerBadge = styleLabel.Render("│ ") + styleGitSynced.Render(fmt.Sprintf("[Peer: %s]", m.peerName()))
		}
		navLine = styleLabel.Render("  ↑↓") + styleValue.Render(" siblings  ") +
			styleLabel.Render("→") + styleValue.Render(" enter dir  ") +
			styleLabel.Render("←") + styleValue.Render(" back  ") +
			styleAction.Render("Enter") + styleValue.Render(" details  ") +
			styleLabel.Render("│ ") +
			styleLabel.Render("[") + styleAction.Render("/") + styleLabel.Render("cmd] ") +
			styleLabel.Render("[") + styleAction.Render("F") + styleLabel.Render("ilter]") + filterBadge + " " +
			styleLabel.Render("[") + styleAction.Render("f") + styleLabel.Render("ile ref] ") +
			styleLabel.Render("[") + styleAction.Render("N") + styleLabel.Render("etwork] ") +
			styleLabel.Render("[") + styleAction.Render("S") + styleLabel.Render("ync] ") +
			styleLabel.Render("[") + styleAction.Render("n") + styleLabel.Render("ame] ") +
			styleLabel.Render("[") + styleAction.Render("r") + styleLabel.Render("efresh] ") +
			styleLabel.Render("[") + styleAction.Render("?") + styleLabel.Render("Help] ") +
			styleLabel.Render("[") + styleAction.Render("Q") + styleLabel.Render("uit]") +
			peerBadge
	}
	}
	statusBar := styleStatusBar.Width(m.width).Render(statusContent + "\n" + navLine)

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
  r              Refresh selected repo (fetch from remote)
  R              Refresh ALL repos (with confirmation)

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

  Peer Sync
  ─────────
  N              Open peer sync menu
  1              Combined view (both perspectives)
  2              Local perspective (actions on this machine)
  3              Remote perspective (actions on peer)
  4              My Tree (normal local view)
  5              Peer Tree (peer's repos, remote actions)
  D              Disconnect from peer

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

func (m Model) renderClonePrompt() string {
	host := ""
	action := "Clone"
	if m.pendingClone != nil {
		host = m.pendingClone.host
		if m.pendingClone.cloneAll {
			action = "Clone all missing repos"
		} else {
			action = fmt.Sprintf("Clone %s", m.pendingClone.relPath)
		}
	}

	prompt := fmt.Sprintf(`
  SSH Clone Available

  SSH access to %s detected.
  %s

  [A] Use SSH for all clones (this session)
  [Y] Use SSH for this clone only
  [H] Use HTTPS instead

  [Esc] Cancel
`, host, action)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(54).
			Padding(1, 2).
			Render(styleHeader.Render(prompt)))
}

func (m Model) renderBusyOverlay() string {
	boxWidth := 46

	var lines []string
	lines = append(lines, "")
	lines = append(lines, styleHeader.Render("  "+m.busyTitle))
	lines = append(lines, "")

	if m.busyTotal > 0 {
		// Progress bar
		barWidth := boxWidth - 8
		if barWidth < 10 {
			barWidth = 10
		}
		progress := float64(m.busyDone) / float64(m.busyTotal)
		filled := int(progress * float64(barWidth))
		if filled > barWidth {
			filled = barWidth
		}
		empty := barWidth - filled

		bar := styleGitSynced.Render(strings.Repeat("█", filled)) +
			styleNonGit.Render(strings.Repeat("░", empty))
		lines = append(lines, "  "+bar)
		lines = append(lines, "")
		lines = append(lines, styleLabel.Render(fmt.Sprintf("  %d / %d repos", m.busyDone, m.busyTotal)))
	} else {
		lines = append(lines, styleLabel.Render("  Working..."))
	}

	if m.busyCurrent != "" {
		lines = append(lines, styleLabel.Render("  Current: ")+styleValue.Render(m.busyCurrent))
	}
	lines = append(lines, "")

	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(boxWidth).
			Padding(1, 2).
			Render(content))
}

func (m Model) renderRenameConfirm() string {
	pr := m.pendingRename
	prompt := fmt.Sprintf(`
  Rename Folder

  Current:  %s
  New name: %s

  The folder will be renamed to match
  the remote repository name.

  [Y] Confirm rename
  [Any other key] Cancel
`, filepath.Base(pr.oldPath), pr.newName)

	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(50).
			Padding(1, 2).
			Render(styleHeader.Render(prompt)))
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
