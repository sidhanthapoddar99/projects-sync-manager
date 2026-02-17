package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/sid/psm/internal/git"
)

const maxWorkers = 10

// TreeNode represents a directory node in the scanned tree.
type TreeNode struct {
	Name      string
	Path      string
	IsGitRepo bool
	Status    *git.RepoStatus
	Children  []*TreeNode
	Parent    *TreeNode
	Expanded  bool
	Depth     int
}

// HasGitDescendant returns true if any child (recursively) is a git repo.
func (n *TreeNode) HasGitDescendant() bool {
	if n.IsGitRepo {
		return true
	}
	for _, c := range n.Children {
		if c.HasGitDescendant() {
			return true
		}
	}
	return false
}

// FlattenVisible returns a flat list of visible nodes for rendering.
func (n *TreeNode) FlattenVisible() []*TreeNode {
	var result []*TreeNode
	result = append(result, n)
	if n.Expanded {
		for _, c := range n.Children {
			result = append(result, c.FlattenVisible()...)
		}
	}
	return result
}

// ScanDirectory scans a directory tree up to maxDepth looking for git repos.
// Non-git directories with no git descendants are shown collapsed.
func ScanDirectory(rootPath string, maxDepth int) *TreeNode {
	absPath, err := filepath.Abs(rootPath)
	if err != nil {
		absPath = rootPath
	}

	root := &TreeNode{
		Name:     filepath.Base(absPath),
		Path:     absPath,
		Expanded: true,
		Depth:    0,
	}

	scanChildren(root, maxDepth)
	fetchStatusesConcurrently(root)

	// Auto-expand folders that contain git repos
	autoExpand(root)

	return root
}

func scanChildren(node *TreeNode, maxDepth int) {
	if node.Depth >= maxDepth {
		return
	}

	entries, err := os.ReadDir(node.Path)
	if err != nil {
		return
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		// Skip hidden directories and common non-project dirs
		if strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "__pycache__" {
			continue
		}

		childPath := filepath.Join(node.Path, name)
		child := &TreeNode{
			Name:      name,
			Path:      childPath,
			IsGitRepo: git.IsGitRepository(childPath),
			Parent:    node,
			Depth:     node.Depth + 1,
		}

		// Don't recurse into git repos (they're leaf nodes)
		if !child.IsGitRepo {
			scanChildren(child, maxDepth)
		}

		node.Children = append(node.Children, child)
	}

	// Sort children: directories with git repos first, then alphabetical
	sort.Slice(node.Children, func(i, j int) bool {
		iHasGit := node.Children[i].HasGitDescendant()
		jHasGit := node.Children[j].HasGitDescendant()
		if iHasGit != jHasGit {
			return iHasGit
		}
		return strings.ToLower(node.Children[i].Name) < strings.ToLower(node.Children[j].Name)
	})
}

// fetchStatusesConcurrently fetches git status for all git repos using a worker pool.
func fetchStatusesConcurrently(root *TreeNode) {
	var repos []*TreeNode
	collectGitRepos(root, &repos)

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)

	for _, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(r *TreeNode) {
			defer wg.Done()
			defer func() { <-sem }()
			r.Status = git.GetRepoStatus(r.Path)
		}(repo)
	}

	wg.Wait()
}

func collectGitRepos(node *TreeNode, repos *[]*TreeNode) {
	if node.IsGitRepo {
		*repos = append(*repos, node)
	}
	for _, c := range node.Children {
		collectGitRepos(c, repos)
	}
}

func autoExpand(node *TreeNode) {
	if node.HasGitDescendant() {
		node.Expanded = true
	}
	for _, c := range node.Children {
		autoExpand(c)
	}
}

// RefreshNode refreshes the git status of a single node.
func RefreshNode(node *TreeNode) {
	if node.IsGitRepo {
		node.Status = git.GetRepoStatus(node.Path)
	}
}

// RefreshAll refreshes all git statuses concurrently.
func RefreshAll(root *TreeNode) {
	fetchStatusesConcurrently(root)
}
