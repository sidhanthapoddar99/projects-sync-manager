package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/network"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// --- Message types ---

type serverReadyMsg struct {
	code     string
	ips      []string
	resultCh chan network.ServerResult
	server   *http.Server
}

type peerConnectedMsg struct {
	peer     *network.Peer
	hostname string
}

type peerMessageMsg struct {
	msg network.Message
}

type peerDisconnectedMsg struct {
	err error
}

type connectFailedMsg struct {
	err error
}

// peerActionDoneMsg is sent when a locally-executed action (triggered by peer request) completes.
type peerActionDoneMsg struct {
	action  string // "clone", "sync"
	path    string
	message string
	err     error
}

// --- Cmds ---

func waitForPeerMsg(ch chan network.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return peerDisconnectedMsg{}
		}
		return peerMessageMsg{msg: msg}
	}
}

func waitForServerClient(ch chan network.ServerResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return peerDisconnectedMsg{}
		}
		return peerConnectedMsg{peer: result.Peer, hostname: result.Hostname}
	}
}

// --- Tab definitions ---
// Tab 0: Combined (both perspectives)
// Tab 1: Local perspective (peer as ref, actions on local)
// Tab 2: Remote perspective (inverted, actions on peer)
// Tab 3: My Tree (normal view of local)
// Tab 4: Peer Tree (normal view of peer's repos)

const peerTabCount = 5

var peerTabNames = [peerTabCount]string{"Combined", "Local", "Remote", "My Tree", "Peer Tree"}

// --- Input handling for network views ---

func (m Model) handleNetworkMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s", "S":
		m.viewMode = ViewNetworkServerWait
		m.networkField = 0
		if m.networkInput[0] == "" {
			m.networkInput[0] = "3000"
		}
		return m, nil
	case "c", "C":
		m.viewMode = ViewNetworkClientInput
		m.networkField = 0
		return m, nil
	case "esc":
		m.viewMode = ViewNormal
		return m, nil
	}
	return m, nil
}

func (m Model) handleNetworkServerWaitKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.httpServer != nil {
		switch msg.String() {
		case "esc":
			network.ShutdownServer(m.httpServer)
			m.httpServer = nil
			m.serverCode = ""
			m.serverIPs = nil
			m.viewMode = ViewNormal
			m.statusText = "Server stopped"
			m.statusError = false
			return m, nil
		}
		return m, nil
	}

	switch msg.Type {
	case tea.KeyEnter:
		port := 3000
		if m.networkInput[0] != "" {
			fmt.Sscanf(m.networkInput[0], "%d", &port)
		}
		code := network.GenerateCode()
		srv, resultCh, err := network.StartServer(port, code)
		if err != nil {
			m.statusText = fmt.Sprintf("Server failed: %v", err)
			m.statusError = true
			m.viewMode = ViewNormal
			return m, nil
		}
		m.httpServer = srv
		m.serverCode = code
		m.serverIPs = network.GetLocalIPs()
		m.statusText = fmt.Sprintf("Server listening on port %d", port)
		m.statusError = false
		return m, waitForServerClient(resultCh)
	case tea.KeyEscape:
		m.viewMode = ViewNormal
		return m, nil
	case tea.KeyBackspace:
		if len(m.networkInput[0]) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.networkInput[0])
			m.networkInput[0] = m.networkInput[0][:len(m.networkInput[0])-size]
		}
		return m, nil
	case tea.KeyRunes:
		for _, r := range msg.Runes {
			if r >= '0' && r <= '9' {
				m.networkInput[0] += string(r)
			}
		}
		return m, nil
	}
	return m, nil
}

func (m Model) handleNetworkClientInputKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyTab:
		m.networkField = (m.networkField + 1) % 2
		return m, nil
	case tea.KeyEnter:
		url := m.networkInput[1]
		code := m.networkInput[2]
		if url == "" || code == "" {
			m.statusText = "URL and code are required"
			m.statusError = true
			return m, nil
		}
		m.statusText = fmt.Sprintf("Connecting to %s...", url)
		m.statusError = false
		return m, func() tea.Msg {
			peer, hostname, err := network.Connect(url, code)
			if err != nil {
				return connectFailedMsg{err: err}
			}
			return peerConnectedMsg{peer: peer, hostname: hostname}
		}
	case tea.KeyEscape:
		m.viewMode = ViewNormal
		return m, nil
	case tea.KeyBackspace:
		idx := m.networkField + 1
		if len(m.networkInput[idx]) > 0 {
			_, size := utf8.DecodeLastRuneInString(m.networkInput[idx])
			m.networkInput[idx] = m.networkInput[idx][:len(m.networkInput[idx])-size]
		}
		return m, nil
	case tea.KeyRunes:
		idx := m.networkField + 1
		m.networkInput[idx] += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

func (m Model) handlePeerCompareKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	// Tab switching (works in all tabs)
	switch msg.String() {
	case "1":
		m.peerCompareTab = 0
		m.rebuildPeerCompareView()
		return m, nil
	case "2":
		m.peerCompareTab = 1
		m.rebuildPeerCompareView()
		return m, nil
	case "3":
		m.peerCompareTab = 2
		m.rebuildPeerCompareView()
		return m, nil
	case "4":
		m.peerCompareTab = 3
		return m, nil
	case "5":
		m.peerCompareTab = 4
		m.rebuildPeerTreeView()
		return m, nil
	case "D":
		m.disconnectPeer()
		m.viewMode = ViewNormal
		m.statusText = "Disconnected from peer"
		m.statusError = false
		return m, nil
	case "esc":
		m.disconnectPeer()
		m.viewMode = ViewNormal
		return m, nil
	}

	// Tab-specific key handling
	switch m.peerCompareTab {
	case 0, 1, 2:
		return m.handlePeerCompareNavKey(msg)
	case 3:
		return m.handlePeerLocalTreeKey(msg)
	case 4:
		return m.handlePeerRemoteTreeKey(msg)
	}
	return m, nil
}

// handlePeerCompareNavKey handles navigation and actions for compare tabs (0-2).
func (m Model) handlePeerCompareNavKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.compareView == nil {
		return m, nil
	}
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
		e := m.compareView.selectedEntry()
		if e == nil {
			return m, nil
		}
		switch e.status {
		case "missing":
			if m.peerCompareTab == 2 {
				// Remote perspective: "missing" means peer doesn't have it → clone on peer
				return m, m.sendRemoteClone(e.path, e.remoteURL)
			}
			// Tab 0 or 1: missing means we don't have it → clone locally
			return m.initiateClone(e.path, e.remoteURL, false)
		case "extra":
			if m.peerCompareTab == 0 {
				// Combined view: "extra" means we have it, peer doesn't → clone on peer
				return m, m.sendRemoteClone(e.path, e.remoteURL)
			}
		}
	case "a":
		// Clone all missing
		var firstMissing *compareEntry
		m.compareView.walkEntries(func(entry *compareEntry) {
			if entry.status == "missing" && firstMissing == nil {
				firstMissing = entry
			}
		})
		if firstMissing == nil {
			return m, nil
		}
		if m.peerCompareTab == 2 {
			// Remote perspective: clone all on peer
			m.statusText = "Requesting peer to clone all missing..."
			m.statusError = false
			m.compareView.walkEntries(func(entry *compareEntry) {
				if entry.status == "missing" {
					go m.peer.Send(network.MsgCloneRequest, network.CloneRequestData{
						Path: entry.path,
						URL:  entry.remoteURL,
					})
				}
			})
			return m, nil
		}
		return m.initiateClone("", firstMissing.remoteURL, true)
	}
	return m, nil
}

// handlePeerLocalTreeKey handles keys for tab 3 (My Tree — normal local view).
func (m Model) handlePeerLocalTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.selectedIdx = navigateUp(m.flatNodes, m.selectedIdx, m.filters.IsActive())
	case "down", "j":
		m.selectedIdx = navigateDown(m.flatNodes, m.selectedIdx, m.filters.IsActive())
	case "right", "l":
		if m.selectedIdx < len(m.flatNodes) {
			targetNode := m.flatNodes[m.selectedIdx]
			newIdx, changed := navigateRight(m.flatNodes, m.selectedIdx)
			if changed {
				m.rebuildFlatList()
				for i, n := range m.flatNodes {
					if n == targetNode {
						if len(targetNode.Children) > 0 {
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
	case "s":
		return m.handleSyncKey()
	case "r":
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
	case "c":
		return m.handleActionKey(handleOpenVSCode)
	case "e":
		return m.handleActionKey(handleOpenExplorer)
	case "b":
		return m.handleBrowserKey()
	}
	return m, nil
}

// handlePeerRemoteTreeKey handles keys for tab 4 (Peer Tree — peer's repos).
func (m Model) handlePeerRemoteTreeKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "up", "k":
		m.peerSelectedIdx = navigateUp(m.peerFlatNodes, m.peerSelectedIdx, false)
	case "down", "j":
		m.peerSelectedIdx = navigateDown(m.peerFlatNodes, m.peerSelectedIdx, false)
	case "right", "l":
		if m.peerSelectedIdx < len(m.peerFlatNodes) {
			targetNode := m.peerFlatNodes[m.peerSelectedIdx]
			newIdx, changed := navigateRight(m.peerFlatNodes, m.peerSelectedIdx)
			if changed {
				m.peerFlatNodes = m.peerRoot.FlattenVisible()
				for i, n := range m.peerFlatNodes {
					if n == targetNode {
						if len(targetNode.Children) > 0 {
							for j, c := range m.peerFlatNodes {
								if c == targetNode.Children[0] {
									m.peerSelectedIdx = j
									break
								}
							}
						} else {
							m.peerSelectedIdx = i
						}
						break
					}
				}
			} else {
				m.peerSelectedIdx = newIdx
			}
		}
	case "left", "h":
		newIdx, changed := navigateLeft(m.peerFlatNodes, m.peerSelectedIdx)
		if changed {
			m.peerFlatNodes = m.peerRoot.FlattenVisible()
		}
		m.peerSelectedIdx = newIdx
	case "s":
		// Sync on peer
		if m.peerSelectedIdx < len(m.peerFlatNodes) {
			node := m.peerFlatNodes[m.peerSelectedIdx]
			if node.IsGitRepo && node.Status != nil && node.Status.HasRemote {
				relPath := node.Path // for peer tree, Path stores relative path
				m.statusText = fmt.Sprintf("Requesting peer to sync %s...", node.Name)
				m.statusError = false
				go m.peer.Send(network.MsgSyncRequest, network.SyncRequestData{Path: relPath})
				return m, nil
			}
		}
	}
	return m, nil
}

// --- Remote action helpers ---

func (m *Model) sendRemoteClone(relPath, remoteURL string) tea.Cmd {
	if m.peer == nil {
		return nil
	}
	m.statusText = fmt.Sprintf("Requesting peer to clone %s...", filepath.Base(relPath))
	m.statusError = false
	peer := m.peer
	return func() tea.Msg {
		_ = peer.Send(network.MsgCloneRequest, network.CloneRequestData{
			Path: relPath,
			URL:  remoteURL,
		})
		return nil
	}
}

// --- Peer connection/disconnection helpers ---

func (m *Model) disconnectPeer() {
	if m.peer != nil {
		_ = m.peer.Send(network.MsgDisconnect, nil)
		m.peer.Close()
		m.peer = nil
	}
	m.peerTree = nil
	m.peerRoot = nil
	m.peerFlatNodes = nil
	m.peerCompareTab = 0
	if m.httpServer != nil {
		network.ShutdownServer(m.httpServer)
		m.httpServer = nil
	}
	m.serverCode = ""
	m.serverIPs = nil
}

// rebuildPeerCompareView rebuilds the compare view from peerTree with current tab.
func (m *Model) rebuildPeerCompareView() {
	if m.peerTree == nil {
		return
	}

	// Tabs 3 and 4 don't use compareView
	if m.peerCompareTab >= 3 {
		return
	}

	ref := network.PeerTreeToRefFile(m.peerTree)
	fullResult := reference.Compare(ref, m.root)

	var result *reference.CompareResult
	switch m.peerCompareTab {
	case 0: // Combined (all)
		result = fullResult
	case 1: // Local perspective (peer as ref, actions on local)
		result = fullResult // same data, different action routing
	case 2: // Remote perspective (inverted — our repos as ref, from peer's viewpoint)
		result = &reference.CompareResult{
			Matched:   fullResult.Matched,
			Relocated: fullResult.Relocated,
		}
		// What's missing locally (peer has, we don't) → peer has it extra
		for _, e := range fullResult.Missing {
			result.Extra = append(result.Extra, &reference.CompareEntry{
				RelativePath: e.RelativePath,
				ActualPath:   e.RelativePath,
				RemoteURL:    e.RemoteURL,
			})
		}
		// What's extra locally (we have, peer doesn't) → peer is missing it
		for _, e := range fullResult.Extra {
			result.Missing = append(result.Missing, &reference.CompareEntry{
				RelativePath: e.RelativePath,
				RemoteURL:    e.RemoteURL,
				LocalNode:    e.LocalNode,
			})
		}
	}

	m.compareView = newCompareViewWithHeader(result, m.rootPath, m.peerCompareHeader())
}

// rebuildPeerTreeView builds the virtual tree for Tab 4 (Peer Tree).
func (m *Model) rebuildPeerTreeView() {
	if m.peerTree == nil {
		return
	}
	m.peerRoot = buildPeerTreeNodes(m.peerTree)
	m.peerFlatNodes = m.peerRoot.FlattenVisible()
	if m.peerSelectedIdx >= len(m.peerFlatNodes) {
		m.peerSelectedIdx = len(m.peerFlatNodes) - 1
	}
	if m.peerSelectedIdx < 0 {
		m.peerSelectedIdx = 0
	}
}

// buildPeerTreeNodes creates a virtual scanner.TreeNode tree from PeerTree data.
func buildPeerTreeNodes(pt *network.PeerTree) *scanner.TreeNode {
	root := &scanner.TreeNode{
		Name:     pt.Hostname,
		Path:     pt.Hostname,
		Expanded: true,
	}

	for _, repo := range pt.Repos {
		parts := strings.Split(repo.RelativePath, string(filepath.Separator))
		current := root
		for j, part := range parts {
			isLast := j == len(parts)-1
			if isLast {
				node := &scanner.TreeNode{
					Name:      part,
					Path:      repo.RelativePath,
					IsGitRepo: true,
					Parent:    current,
					Expanded:  false,
					Depth:     current.Depth + 1,
					Status:    fakePeerRepoStatus(repo),
				}
				current.Children = append(current.Children, node)
			} else {
				var found *scanner.TreeNode
				for _, c := range current.Children {
					if c.Name == part && !c.IsGitRepo {
						found = c
						break
					}
				}
				if found == nil {
					found = &scanner.TreeNode{
						Name:     part,
						Path:     filepath.Join(current.Path, part),
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
	sortTreeNodes(root)
	return root
}

func sortTreeNodes(node *scanner.TreeNode) {
	sort.Slice(node.Children, func(i, j int) bool {
		iDir := !node.Children[i].IsGitRepo
		jDir := !node.Children[j].IsGitRepo
		if iDir != jDir {
			return iDir
		}
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})
	for _, c := range node.Children {
		sortTreeNodes(c)
	}
}

// fakePeerRepoStatus creates a RepoStatus that produces the correct SyncState.
func fakePeerRepoStatus(repo network.PeerRepo) *git.RepoStatus {
	s := &git.RepoStatus{
		IsGitRepo:   true,
		HasRemote:   true,
		RemoteURL:   repo.RemoteURL,
		HTTPSURL:    repo.RemoteURL,
		TotalAhead:  repo.Ahead,
		TotalBehind: repo.Behind,
		RepoName:    filepath.Base(repo.RelativePath),
	}
	// Force correct SyncState via fields
	switch git.SyncState(repo.SyncState) {
	case git.StateDirty:
		s.Staged = 1 // triggers StateDirty in SyncState()
	case git.StateNoRemote:
		s.HasRemote = false
	case git.StateNotGit:
		s.IsGitRepo = false
	}
	// StatePartial and StateSynced are naturally computed from TotalAhead/TotalBehind
	return s
}

func (m *Model) peerCompareHeader() string {
	host := "Peer"
	if m.peerTree != nil {
		host = m.peerTree.Hostname
	}
	var parts []string
	for i, t := range peerTabNames {
		if i == m.peerCompareTab {
			parts = append(parts, styleAction.Render(fmt.Sprintf("[%d:%s]", i+1, t)))
		} else {
			parts = append(parts, styleLabel.Render(fmt.Sprintf(" %d:%s ", i+1, t)))
		}
	}
	return fmt.Sprintf("Peer: %s  %s", host, strings.Join(parts, " "))
}

// renderPeerTabBar renders the tab bar for all 5 views.
func (m *Model) renderPeerTabBar(width int) string {
	host := "Peer"
	if m.peerTree != nil {
		host = m.peerTree.Hostname
	}
	var parts []string
	for i, t := range peerTabNames {
		if i == m.peerCompareTab {
			parts = append(parts, styleAction.Render(fmt.Sprintf("[%d:%s]", i+1, t)))
		} else {
			parts = append(parts, styleLabel.Render(fmt.Sprintf(" %d:%s ", i+1, t)))
		}
	}
	return styleHeader.Render("  "+host) + "  " + strings.Join(parts, "")
}

// sendTreeToPeer sends current local tree to connected peer.
func (m *Model) sendTreeToPeer() {
	if m.peer == nil || m.root == nil {
		return
	}
	tree := network.PeerTreeFromRoot(m.root)
	go m.peer.SendTree(tree)
}

// peerName returns the peer's hostname for display.
func (m *Model) peerName() string {
	if m.peer != nil {
		return m.peer.Hostname
	}
	return ""
}

// handlePeerMessage processes an incoming message from the peer.
func (m *Model) handlePeerMessage(msg network.Message) tea.Cmd {
	switch msg.Type {
	case network.MsgTree:
		var pt network.PeerTree
		if err := json.Unmarshal(msg.Data, &pt); err != nil {
			return nil
		}
		m.peerTree = &pt
		if m.peerCompareTab <= 2 {
			m.rebuildPeerCompareView()
		} else if m.peerCompareTab == 4 {
			m.rebuildPeerTreeView()
		}
		if m.viewMode != ViewPeerCompare {
			m.viewMode = ViewPeerCompare
		}

	case network.MsgDisconnect:
		m.peer.Close()
		m.peer = nil
		m.peerTree = nil
		m.peerRoot = nil
		m.peerFlatNodes = nil
		m.httpServer = nil
		m.serverCode = ""
		m.viewMode = ViewNormal
		m.statusText = "Peer disconnected"
		m.statusError = false

	case network.MsgCloneRequest:
		// Peer is asking us to clone a repo
		var req network.CloneRequestData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil
		}
		m.statusText = fmt.Sprintf("Peer requested clone of %s...", filepath.Base(req.Path))
		m.statusError = false
		rootPath := m.rootPath
		return func() tea.Msg {
			targetPath := filepath.Join(rootPath, req.Path)
			err := git.CloneRepo(req.URL, targetPath)
			msg := ""
			if err == nil {
				msg = fmt.Sprintf("Cloned %s", req.Path)
			}
			return peerActionDoneMsg{action: "clone", path: req.Path, message: msg, err: err}
		}

	case network.MsgSyncRequest:
		// Peer is asking us to sync a repo
		var req network.SyncRequestData
		if err := json.Unmarshal(msg.Data, &req); err != nil {
			return nil
		}
		m.statusText = fmt.Sprintf("Peer requested sync of %s...", filepath.Base(req.Path))
		m.statusError = false
		rootPath := m.rootPath
		return func() tea.Msg {
			targetPath := filepath.Join(rootPath, req.Path)
			result := git.SyncRepo(targetPath)
			return peerActionDoneMsg{action: "sync", path: req.Path, message: result.Message, err: result.Err}
		}

	case network.MsgActionResult:
		// Peer reports completion of an action we requested
		var res network.ActionResultData
		if err := json.Unmarshal(msg.Data, &res); err != nil {
			return nil
		}
		if res.Error != "" {
			m.statusText = fmt.Sprintf("Remote %s failed: %s: %s", res.Action, filepath.Base(res.Path), res.Error)
			m.statusError = true
		} else {
			m.statusText = fmt.Sprintf("Remote %s: %s", res.Action, res.Message)
			m.statusError = false
		}
	}
	return nil
}

// --- Rendering ---

func (m Model) renderNetworkMenu() string {
	menu := `
  Peer Sync

  [S] Start Server
  [C] Connect to Peer

  [Esc] Cancel
`
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(34).
			Padding(1, 2).
			Render(styleHeader.Render(menu)))
}

// bigChar maps characters to 5-line block pixel art using double-width ASCII.
var bigChar = map[byte][5]string{
	'0': {"######", "##  ##", "##  ##", "##  ##", "######"},
	'1': {"  ##  ", "####  ", "  ##  ", "  ##  ", "######"},
	'2': {"######", "    ##", "######", "##    ", "######"},
	'3': {"######", "    ##", "######", "    ##", "######"},
	'4': {"##  ##", "##  ##", "######", "    ##", "    ##"},
	'5': {"######", "##    ", "######", "    ##", "######"},
	'6': {"######", "##    ", "######", "##  ##", "######"},
	'7': {"######", "    ##", "  ##  ", "  ##  ", "  ##  "},
	'8': {"######", "##  ##", "######", "##  ##", "######"},
	'9': {"######", "##  ##", "######", "    ##", "######"},
	'A': {"  ##  ", "##  ##", "######", "##  ##", "##  ##"},
	'B': {"####  ", "##  ##", "####  ", "##  ##", "####  "},
	'C': {"######", "##    ", "##    ", "##    ", "######"},
	'D': {"####  ", "##  ##", "##  ##", "##  ##", "####  "},
	'E': {"######", "##    ", "####  ", "##    ", "######"},
	'F': {"######", "##    ", "####  ", "##    ", "##    "},
	'G': {"######", "##    ", "##  ##", "##  ##", "######"},
	'H': {"##  ##", "##  ##", "######", "##  ##", "##  ##"},
	'J': {"    ##", "    ##", "    ##", "##  ##", "######"},
	'K': {"##  ##", "####  ", "##    ", "####  ", "##  ##"},
	'L': {"##    ", "##    ", "##    ", "##    ", "######"},
	'M': {"##  ##", "######", "######", "##  ##", "##  ##"},
	'N': {"##  ##", "######", "######", "##  ##", "##  ##"},
	'P': {"######", "##  ##", "######", "##    ", "##    "},
	'Q': {"######", "##  ##", "##  ##", "####  ", "  ####"},
	'R': {"######", "##  ##", "####  ", "##  ##", "##  ##"},
	'S': {"######", "##    ", "######", "    ##", "######"},
	'T': {"######", "  ##  ", "  ##  ", "  ##  ", "  ##  "},
	'U': {"##  ##", "##  ##", "##  ##", "##  ##", "######"},
	'V': {"##  ##", "##  ##", "##  ##", "##  ##", "  ##  "},
	'W': {"##  ##", "##  ##", "######", "######", "##  ##"},
	'X': {"##  ##", "##  ##", "  ##  ", "##  ##", "##  ##"},
	'Y': {"##  ##", "##  ##", "  ##  ", "  ##  ", "  ##  "},
	'Z': {"######", "    ##", "  ##  ", "##    ", "######"},
}

func renderBigCode(code string) string {
	if len(code) == 0 {
		return ""
	}

	glyphs := make([][5]string, len(code))
	for i := 0; i < len(code); i++ {
		g, ok := bigChar[code[i]]
		if !ok {
			g = [5]string{"   ", "   ", " ? ", "   ", "   "}
		}
		glyphs[i] = g
	}

	const innerW = 8
	gap := "  "

	var top strings.Builder
	for i := range glyphs {
		if i > 0 {
			top.WriteString(gap)
		}
		top.WriteString("┌")
		top.WriteString(strings.Repeat("─", innerW))
		top.WriteString("┐")
	}

	var bot strings.Builder
	for i := range glyphs {
		if i > 0 {
			bot.WriteString(gap)
		}
		bot.WriteString("└")
		bot.WriteString(strings.Repeat("─", innerW))
		bot.WriteString("┘")
	}

	rows := make([]string, 5)
	for r := 0; r < 5; r++ {
		var line strings.Builder
		for i, g := range glyphs {
			if i > 0 {
				line.WriteString(gap)
			}
			line.WriteString(styleLabel.Render("│"))
			line.WriteString(" ")
			line.WriteString(styleAction.Render(g[r]))
			line.WriteString(" ")
			line.WriteString(styleLabel.Render("│"))
		}
		rows[r] = line.String()
	}

	var result []string
	result = append(result, styleLabel.Render(top.String()))
	result = append(result, rows...)
	result = append(result, styleLabel.Render(bot.String()))

	return strings.Join(result, "\n")
}

func (m Model) renderNetworkServerWait() string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, styleHeader.Render("  Server Running"))
	lines = append(lines, "")

	if m.httpServer == nil {
		lines = append(lines, styleLabel.Render("  Port: ")+styleValue.Render(m.networkInput[0])+styleLabel.Render("│"))
		lines = append(lines, "")
		lines = append(lines, styleAction.Render("  [Enter] Start server"))
		lines = append(lines, styleLabel.Render("  [Esc] Cancel"))

		content := strings.Join(lines, "\n")
		return lipgloss.Place(m.width, m.height,
			lipgloss.Center, lipgloss.Center,
			stylePanelBorder.
				Width(40).
				Padding(1, 2).
				Render(content))
	}

	lines = append(lines, styleLabel.Render("  Connection Code:"))
	lines = append(lines, "")

	bigCode := renderBigCode(m.serverCode)
	lines = append(lines, bigCode)

	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Port: ")+styleValue.Render(m.networkInput[0]))
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Share with your peer:"))
	for _, ip := range m.serverIPs {
		lines = append(lines, styleInfo.Render(fmt.Sprintf("    %s:%s", ip, m.networkInput[0])))
	}
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Waiting for client..."))
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  [Esc] Stop server"))

	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(56).
			Padding(1, 2).
			Render(content))
}

func (m Model) renderNetworkClientInput() string {
	var lines []string
	lines = append(lines, "")
	lines = append(lines, styleHeader.Render("  Connect to Peer"))
	lines = append(lines, "")

	urlStyle := styleValue
	codeStyle := styleValue
	if m.networkField == 0 {
		urlStyle = styleAction
	} else {
		codeStyle = styleAction
	}

	urlCursor := " "
	codeCursor := " "
	if m.networkField == 0 {
		urlCursor = "│"
	} else {
		codeCursor = "│"
	}

	lines = append(lines, styleLabel.Render("  URL:  ")+urlStyle.Render(m.networkInput[1])+styleLabel.Render(urlCursor))
	lines = append(lines, styleLabel.Render("  Code: ")+codeStyle.Render(m.networkInput[2])+styleLabel.Render(codeCursor))
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Tab to switch fields"))
	lines = append(lines, styleAction.Render("  [Enter] Connect"))
	lines = append(lines, styleLabel.Render("  [Esc] Cancel"))

	content := strings.Join(lines, "\n")
	return lipgloss.Place(m.width, m.height,
		lipgloss.Center, lipgloss.Center,
		stylePanelBorder.
			Width(40).
			Padding(1, 2).
			Render(content))
}

// renderPeerActions renders actions for peer tree nodes (tab 4).
func renderPeerActions(node *scanner.TreeNode, width int) string {
	if node == nil {
		return ""
	}

	var lines []string
	lines = append(lines, styleSection.Render("  Remote Actions"))
	lines = append(lines, "")

	if node.IsGitRepo && node.Status != nil && node.Status.HasRemote {
		lines = append(lines, styleAction.Render("  [S] Sync on peer"))
	}
	lines = append(lines, "")
	lines = append(lines, styleLabel.Render("  Actions execute on the peer's machine"))

	return strings.Join(lines, "\n")
}
