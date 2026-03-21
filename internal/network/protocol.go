package network

import (
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/reference"
	"github.com/sid/psm/internal/scanner"
)

// Message types exchanged over WebSocket.
const (
	MsgAuth       = "auth"
	MsgAuthOK     = "auth_ok"
	MsgAuthFail   = "auth_fail"
	MsgTree       = "tree"
	MsgDisconnect = "disconnect"

	// Remote action requests and results
	MsgCloneRequest  = "clone_req"
	MsgCloneResult   = "clone_res"
	MsgSyncRequest   = "sync_req"
	MsgActionResult  = "action_res"
)

// Message is the envelope for all WebSocket messages.
type Message struct {
	Type string          `json:"type"`
	Data json.RawMessage `json:"data,omitempty"`
}

// AuthData is sent by the client to authenticate.
type AuthData struct {
	Code     string `json:"code"`
	Hostname string `json:"hostname"`
}

// CloneRequestData asks the peer to clone a repo.
type CloneRequestData struct {
	Path string `json:"path"`
	URL  string `json:"url"`
}

// SyncRequestData asks the peer to sync a repo.
type SyncRequestData struct {
	Path string `json:"path"`
}

// ActionResultData is sent back after a remote action completes.
type ActionResultData struct {
	Action  string `json:"action"`
	Path    string `json:"path"`
	Message string `json:"message,omitempty"`
	Error   string `json:"error,omitempty"`
}

// AuthOKData is sent by the server on successful auth.
type AuthOKData struct {
	Hostname string `json:"hostname"`
}

// PeerRepo represents a single repository in a peer's tree.
type PeerRepo struct {
	RelativePath string `json:"path"`
	RemoteURL    string `json:"url"`
	SyncState    int    `json:"state"`
	Ahead        int    `json:"ahead"`
	Behind       int    `json:"behind"`
}

// PeerTree is the full tree payload exchanged between peers.
type PeerTree struct {
	Hostname string     `json:"host"`
	Repos    []PeerRepo `json:"repos"`
}

// GenerateCode creates a random 4-character alphanumeric code.
func GenerateCode() string {
	const chars = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I
	b := make([]byte, 4)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = chars[int(b[i])%len(chars)]
	}
	return string(b)
}

// PeerTreeFromRoot walks a scanner tree and builds a PeerTree.
func PeerTreeFromRoot(root *scanner.TreeNode) *PeerTree {
	hostname, _ := os.Hostname()
	pt := &PeerTree{Hostname: hostname}

	var collect func(node *scanner.TreeNode)
	collect = func(node *scanner.TreeNode) {
		if node.IsGitRepo && node.Status != nil && node.Status.HasRemote {
			relPath, _ := filepath.Rel(root.Path, node.Path)
			pr := PeerRepo{
				RelativePath: relPath,
				RemoteURL:    git.SSHToHTTPS(node.Status.RemoteURL),
				SyncState:    int(node.Status.SyncState()),
				Ahead:        node.Status.TotalAhead,
				Behind:       node.Status.TotalBehind,
			}
			pt.Repos = append(pt.Repos, pr)
		}
		for _, c := range node.Children {
			collect(c)
		}
	}
	collect(root)
	return pt
}

// PeerTreeToRefFile converts a PeerTree to a RefFile for reuse with reference.Compare().
func PeerTreeToRefFile(pt *PeerTree) *reference.RefFile {
	ref := &reference.RefFile{
		Version:  1,
		BasePath: pt.Hostname,
	}
	for _, r := range pt.Repos {
		ref.Repositories = append(ref.Repositories, reference.RefRepo{
			RelativePath: r.RelativePath,
			RemoteURL:    r.RemoteURL,
		})
	}
	return ref
}

// EncodeMessage serializes a Message to JSON bytes.
func EncodeMessage(msgType string, data interface{}) ([]byte, error) {
	msg := Message{Type: msgType}
	if data != nil {
		raw, err := json.Marshal(data)
		if err != nil {
			return nil, err
		}
		msg.Data = raw
	}
	return json.Marshal(msg)
}

// DecodeMessage deserializes a JSON message.
func DecodeMessage(raw []byte) (*Message, error) {
	var msg Message
	if err := json.Unmarshal(raw, &msg); err != nil {
		return nil, fmt.Errorf("invalid message: %w", err)
	}
	return &msg, nil
}
