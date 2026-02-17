package scanner

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/sid/psm/internal/git"
)

const maxWorkers = 10

// ScanProgress reports scanning progress back to the caller.
type ScanProgress struct {
	Phase          string // "dirs" or "status"
	DirsScanned    int
	DirsTotal      int // 0 if unknown
	ReposFound     int
	ReposFetched   int
	ReposTotal     int
	CurrentDir     string
}

// ProgressFunc is a callback invoked during scanning to report progress.
type ProgressFunc func(ScanProgress)

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
// If onProgress is non-nil, it's called periodically with scan progress.
func ScanDirectory(rootPath string, maxDepth int, onProgress ProgressFunc) *TreeNode {
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

	// Load ignore patterns from .psmignore + built-in defaults
	ignore := loadIgnoreList(absPath)

	// Phase 1: scan directory tree
	var dirsScanned int32
	var reposFound int32
	scanChildren(root, maxDepth, ignore, &dirsScanned, &reposFound, onProgress)

	// Phase 2: fetch git statuses concurrently
	fetchStatusesConcurrently(root, onProgress)

	// Auto-expand folders that contain git repos
	autoExpand(root)

	return root
}

func scanChildren(node *TreeNode, maxDepth int, ignore *ignoreList, dirsScanned, reposFound *int32, onProgress ProgressFunc) {
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

		// Check against ignore list (built-in + .psmignore)
		if ignore.shouldIgnore(name) {
			continue
		}

		childPath := filepath.Join(node.Path, name)
		isGit := git.IsGitRepository(childPath)

		child := &TreeNode{
			Name:      name,
			Path:      childPath,
			IsGitRepo: isGit,
			Parent:    node,
			Depth:     node.Depth + 1,
		}

		atomic.AddInt32(dirsScanned, 1)
		if isGit {
			atomic.AddInt32(reposFound, 1)
		}

		// Report progress
		if onProgress != nil {
			onProgress(ScanProgress{
				Phase:       "dirs",
				DirsScanned: int(atomic.LoadInt32(dirsScanned)),
				ReposFound:  int(atomic.LoadInt32(reposFound)),
				CurrentDir:  name,
			})
		}

		// Stop recursing once a git repo is found — it's a leaf
		if !isGit {
			scanChildren(child, maxDepth, ignore, dirsScanned, reposFound, onProgress)
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
func fetchStatusesConcurrently(root *TreeNode, onProgress ProgressFunc) {
	var repos []*TreeNode
	collectGitRepos(root, &repos)

	total := len(repos)
	var fetched int32

	var wg sync.WaitGroup
	sem := make(chan struct{}, maxWorkers)

	for _, repo := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(r *TreeNode) {
			defer wg.Done()
			defer func() { <-sem }()
			r.Status = git.GetRepoStatus(r.Path)

			done := int(atomic.AddInt32(&fetched, 1))
			if onProgress != nil {
				onProgress(ScanProgress{
					Phase:        "status",
					ReposFetched: done,
					ReposTotal:   total,
					CurrentDir:   r.Name,
				})
			}
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
	fetchStatusesConcurrently(root, nil)
}
