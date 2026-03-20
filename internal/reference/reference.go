package reference

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sid/psm/internal/git"
	"github.com/sid/psm/internal/scanner"
)

// RefFile is the portable reference file format.
type RefFile struct {
	Version      int       `json:"version"`
	CreatedAt    time.Time `json:"created_at"`
	BasePath     string    `json:"base_path"`
	Repositories []RefRepo `json:"repositories"`
}

// RefRepo is a single repository entry in a reference file.
type RefRepo struct {
	RelativePath string `json:"relative_path"`
	RemoteURL    string `json:"remote_url"`
}

// CompareResult holds the result of comparing a reference file against local state.
type CompareResult struct {
	Matched   []*CompareEntry
	Missing   []*CompareEntry // in reference but not local
	Extra     []*CompareEntry // local but not in reference
	Relocated []*CompareEntry // in reference, found locally but at different path
}

// CompareEntry is a single entry in comparison results.
type CompareEntry struct {
	RelativePath string             // expected path (from reference)
	ActualPath   string             // actual local path (differs from RelativePath if relocated)
	RemoteURL    string             // normalized HTTPS URL
	LocalNode    *scanner.TreeNode  // nil if missing locally
	Relocated    bool               // true if found at a different path
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

// normalizeURL strips trailing slashes and .git suffix for comparison.
func normalizeURL(url string) string {
	url = strings.TrimSuffix(url, "/")
	url = strings.TrimSuffix(url, ".git")
	url = strings.ToLower(url)
	return url
}

// Compare compares a reference file against a scanned tree.
// Matching is done by remote URL first (the repo identity), not by path.
// If a repo is found locally but at a different relative path, it is marked as relocated.
func Compare(ref *RefFile, root *scanner.TreeNode) *CompareResult {
	result := &CompareResult{}

	// Build map of local repos by normalized URL → (relPath, node)
	type localRepo struct {
		relPath string
		node    *scanner.TreeNode
	}
	localByURL := make(map[string]*localRepo)
	var collect func(node *scanner.TreeNode)
	collect = func(node *scanner.TreeNode) {
		if node.IsGitRepo && node.Status != nil && node.Status.HasRemote {
			relPath, _ := filepath.Rel(root.Path, node.Path)
			normURL := normalizeURL(git.SSHToHTTPS(node.Status.RemoteURL))
			localByURL[normURL] = &localRepo{relPath: relPath, node: node}
		}
		for _, c := range node.Children {
			collect(c)
		}
	}
	collect(root)

	// Track which local URLs have been matched to a ref entry
	matchedURLs := make(map[string]bool)

	// Check each ref entry against local repos by URL
	for _, repo := range ref.Repositories {
		refNormURL := normalizeURL(repo.RemoteURL)

		if local, exists := localByURL[refNormURL]; exists {
			matchedURLs[refNormURL] = true

			if local.relPath == repo.RelativePath {
				// Same URL, same path → matched
				result.Matched = append(result.Matched, &CompareEntry{
					RelativePath: repo.RelativePath,
					ActualPath:   local.relPath,
					RemoteURL:    repo.RemoteURL,
					LocalNode:    local.node,
				})
			} else {
				// Same URL, different path → relocated
				result.Relocated = append(result.Relocated, &CompareEntry{
					RelativePath: repo.RelativePath,
					ActualPath:   local.relPath,
					RemoteURL:    repo.RemoteURL,
					LocalNode:    local.node,
					Relocated:    true,
				})
			}
		} else {
			// Not found locally by URL → missing
			result.Missing = append(result.Missing, &CompareEntry{
				RelativePath: repo.RelativePath,
				RemoteURL:    repo.RemoteURL,
			})
		}
	}

	// Find extra local repos not matched to any ref entry
	for normURL, local := range localByURL {
		if !matchedURLs[normURL] {
			url := ""
			if local.node.Status != nil {
				url = local.node.Status.HTTPSURL
			}
			result.Extra = append(result.Extra, &CompareEntry{
				RelativePath: local.relPath,
				ActualPath:   local.relPath,
				RemoteURL:    url,
				LocalNode:    local.node,
			})
		}
	}

	return result
}
