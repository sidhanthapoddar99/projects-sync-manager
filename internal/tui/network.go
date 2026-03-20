package tui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"unicode/utf8"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/sid/psm/internal/network"
	"github.com/sid/psm/internal/reference"
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

// --- Cmds ---

// waitForPeerMsg blocks on the peer's InCh and returns messages to bubbletea.
func waitForPeerMsg(ch chan network.Message) tea.Cmd {
	return func() tea.Msg {
		msg, ok := <-ch
		if !ok {
			return peerDisconnectedMsg{}
		}
		return peerMessageMsg{msg: msg}
	}
}

// waitForServerClient blocks until a client connects to the server.
func waitForServerClient(ch chan network.ServerResult) tea.Cmd {
	return func() tea.Msg {
		result, ok := <-ch
		if !ok {
			return peerDisconnectedMsg{}
		}
		return peerConnectedMsg{peer: result.Peer, hostname: result.Hostname}
	}
}

// --- Input handling for network views ---

func (m Model) handleNetworkMenuKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "s", "S":
		// Start server — switch to server input
		m.viewMode = ViewNetworkServerWait
		m.networkField = 0
		if m.networkInput[0] == "" {
			m.networkInput[0] = "3000" // default port
		}
		return m, nil
	case "c", "C":
		// Connect to peer
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
	// If server is already running, only esc to stop
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

	// Port input mode
	switch msg.Type {
	case tea.KeyEnter:
		// Start the server
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
		// Only accept digits for port
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
		m.networkField = (m.networkField + 1) % 2 // toggle between URL(0) and Code(1)
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
		// Connect async
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
		idx := m.networkField + 1 // networkInput[1]=url, [2]=code
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
	switch msg.String() {
	case "up", "k":
		m.compareView.navigateUp()
	case "down", "j":
		m.compareView.navigateDown()
	case "right", "l":
		m.compareView.navigateRight()
	case "left", "h":
		m.compareView.navigateLeft()
	case "1":
		m.peerCompareTab = 0
		m.rebuildPeerCompareView()
	case "2":
		m.peerCompareTab = 1
		m.rebuildPeerCompareView()
	case "3":
		m.peerCompareTab = 2
		m.rebuildPeerCompareView()
	case "enter":
		// Clone single missing repo
		if m.compareView != nil {
			e := m.compareView.selectedEntry()
			if e == nil || e.status != "missing" {
				return m, nil
			}
			return m.initiateClone(e.path, e.remoteURL, false)
		}
	case "a":
		// Clone all missing
		if m.compareView != nil {
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
		}
	case "D":
		// Disconnect
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
	return m, nil
}

// --- Peer connection/disconnection helpers ---

func (m *Model) disconnectPeer() {
	if m.peer != nil {
		_ = m.peer.Send(network.MsgDisconnect, nil)
		m.peer.Close()
		m.peer = nil
	}
	m.peerTree = nil
	m.peerCompareTab = 0
	if m.httpServer != nil {
		network.ShutdownServer(m.httpServer)
		m.httpServer = nil
	}
	m.serverCode = ""
	m.serverIPs = nil
}

// rebuildPeerCompareView rebuilds the compare view from peerTree with current tab filter.
func (m *Model) rebuildPeerCompareView() {
	if m.peerTree == nil {
		return
	}
	ref := network.PeerTreeToRefFile(m.peerTree)
	fullResult := reference.Compare(ref, m.root)

	var result *reference.CompareResult
	switch m.peerCompareTab {
	case 1: // Local only (matched + extra)
		result = &reference.CompareResult{
			Matched: fullResult.Matched,
			Extra:   fullResult.Extra,
		}
	case 2: // Remote only (matched + missing)
		result = &reference.CompareResult{
			Matched: fullResult.Matched,
			Missing: fullResult.Missing,
		}
	default: // Combined (all)
		result = fullResult
	}

	m.compareView = newCompareViewWithHeader(result, m.rootPath, m.peerCompareHeader())
}

func (m *Model) peerCompareHeader() string {
	host := "Peer"
	if m.peerTree != nil {
		host = m.peerTree.Hostname
	}
	tabs := []string{"Combined", "Local", "Remote"}
	var parts []string
	for i, t := range tabs {
		if i == m.peerCompareTab {
			parts = append(parts, styleAction.Render(fmt.Sprintf("[%d:%s]", i+1, t)))
		} else {
			parts = append(parts, styleLabel.Render(fmt.Sprintf(" %d:%s ", i+1, t)))
		}
	}
	return fmt.Sprintf("Peer: %s  %s", host, strings.Join(parts, " "))
}

// sendTreeToPeer sends current local tree to connected peer.
func (m *Model) sendTreeToPeer() {
	if m.peer == nil || m.root == nil {
		return
	}
	tree := network.PeerTreeFromRoot(m.root)
	go m.peer.SendTree(tree)
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
		m.rebuildPeerCompareView()
		if m.viewMode != ViewPeerCompare {
			m.viewMode = ViewPeerCompare
		}
	case network.MsgDisconnect:
		m.peer.Close()
		m.peer = nil
		m.peerTree = nil
		m.httpServer = nil
		m.serverCode = ""
		m.viewMode = ViewNormal
		m.statusText = "Peer disconnected"
		m.statusError = false
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
// Each glyph is 5 visual columns wide (using "##" for on-pixels, "  " for off-pixels,
// so each logical pixel = 2 chars). 3 logical pixels wide × 5 tall = 6 chars per row.
// We use 2-char-per-pixel for wider, more readable display.
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

// renderBigCode renders a 4-char code as large block pixel characters inside boxes.
// Each glyph is 3 cells wide visually (█ = 1 cell). Box inner width = 5 (space + 3 glyph + space).
func renderBigCode(code string) string {
	if len(code) == 0 {
		return ""
	}

	// Collect glyphs for each character
	glyphs := make([][5]string, len(code))
	for i := 0; i < len(code); i++ {
		g, ok := bigChar[code[i]]
		if !ok {
			g = [5]string{"   ", "   ", " ? ", "   ", "   "}
		}
		glyphs[i] = g
	}

	const innerW = 8 // " " + 6-char glyph + " "
	gap := "  "

	// Top border
	var top strings.Builder
	for i := range glyphs {
		if i > 0 {
			top.WriteString(gap)
		}
		top.WriteString("┌")
		top.WriteString(strings.Repeat("─", innerW))
		top.WriteString("┐")
	}

	// Bottom border
	var bot strings.Builder
	for i := range glyphs {
		if i > 0 {
			bot.WriteString(gap)
		}
		bot.WriteString("└")
		bot.WriteString(strings.Repeat("─", innerW))
		bot.WriteString("┘")
	}

	// Content rows (5 rows)
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
		// Still in port input mode
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

	// Server is running — show big code display
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
