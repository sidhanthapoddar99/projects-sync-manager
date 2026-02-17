package reference

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// RefFile is the portable reference file format.
type RefFile struct {
	Version      int         `json:"version"`
	CreatedAt    time.Time   `json:"created_at"`
	BasePath     string      `json:"base_path"`
	Repositories []RefRepo   `json:"repositories"`
}

// RefRepo is a single repository entry in a reference file.
type RefRepo struct {
	RelativePath string `json:"relative_path"`
	RemoteURL    string `json:"remote_url"`
}

// CompareResult holds the result of comparing a reference file against local state.
type CompareResult struct {
	Matched []*CompareEntry
	Missing []*CompareEntry // in reference but not local
	Extra   []*CompareEntry // local but not in reference
}

// CompareEntry is a single entry in comparison results.
type CompareEntry struct {
	RelativePath string
	RemoteURL    string
	LocalNode    *scanner.TreeNode // nil if missing locally
}

// Generate creates a reference file from a scanned tree.
func Generate(root *scanner.TreeNode) *RefFile {
	ref := &RefFile{
		Version:   1,
		CreatedAt: time.Now().UTC(),
		BasePath:  root.Path,
	}

	var collect func(node *scanner.TreeNode)
	collect = func(node *scanner.TreeNode) {
		if node.IsGitRepo && node.Status != nil && node.Status.HasRemote {
			relPath, _ := filepath.Rel(root.Path, node.Path)
			ref.Repositories = append(ref.Repositories, RefRepo{
				RelativePath: relPath,
				RemoteURL:    git.SSHToHTTPS(node.Status.RemoteURL),
			})
		}
		for _, c := range node.Children {
			collect(c)
		}
	}
	collect(root)
	return ref
}

// Save writes a reference file to disk.
func Save(ref *RefFile, filePath string) error {
	data, err := json.MarshalIndent(ref, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filePath, data, 0644)
}

// Load reads a reference file from disk.
func Load(filePath string) (*RefFile, error) {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return nil, err
	}
	var ref RefFile
	if err := json.Unmarshal(data, &ref); err != nil {
		return nil, err
	}
	return &ref, nil
}

// Compare compares a reference file against a scanned tree.
func Compare(ref *RefFile, root *scanner.TreeNode) *CompareResult {
	result := &CompareResult{}

	// Build map of local repos by relative path
	localRepos := make(map[string]*scanner.TreeNode)
	var collect func(node *scanner.TreeNode)
	collect = func(node *scanner.TreeNode) {
		if node.IsGitRepo {
			relPath, _ := filepath.Rel(root.Path, node.Path)
			localRepos[relPath] = node
		}
		for _, c := range node.Children {
			collect(c)
		}
	}
	collect(root)

	// Check each ref entry against local
	refPaths := make(map[string]bool)
	for _, repo := range ref.Repositories {
		refPaths[repo.RelativePath] = true
		if localNode, exists := localRepos[repo.RelativePath]; exists {
			result.Matched = append(result.Matched, &CompareEntry{
				RelativePath: repo.RelativePath,
				RemoteURL:    repo.RemoteURL,
				LocalNode:    localNode,
			})
		} else {
			result.Missing = append(result.Missing, &CompareEntry{
				RelativePath: repo.RelativePath,
				RemoteURL:    repo.RemoteURL,
			})
		}
	}

	// Find extra local repos not in reference
	for relPath, node := range localRepos {
		if !refPaths[relPath] {
			url := ""
			if node.Status != nil {
				url = node.Status.HTTPSURL
			}
			result.Extra = append(result.Extra, &CompareEntry{
				RelativePath: relPath,
				RemoteURL:    url,
				LocalNode:    node,
			})
		}
	}

	return result
}
