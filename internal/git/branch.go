package git

import (
	"strconv"
	"strings"
)

// BranchStatus represents the sync status of a single branch.
type BranchStatus struct {
	Name      string
	IsLocal   bool
	IsRemote  bool
	IsCurrent bool
	Ahead     int
	Behind    int
}

// StatusLabel returns a human-readable status label.
func (b *BranchStatus) StatusLabel() string {
	if !b.IsLocal && b.IsRemote {
		return "remote only"
	}
	if b.IsLocal && !b.IsRemote {
		return "local only"
	}
	if b.Ahead > 0 && b.Behind > 0 {
		return "↑" + strconv.Itoa(b.Ahead) + " ↓" + strconv.Itoa(b.Behind) + " diverged"
	}
	if b.Ahead > 0 {
		return "↑" + strconv.Itoa(b.Ahead) + " ahead"
	}
	if b.Behind > 0 {
		return "↓" + strconv.Itoa(b.Behind) + " behind"
	}
	return "✓ synced"
}

// GetBranchStatuses returns the status of all branches in a repo.
func GetBranchStatuses(repoPath string) []BranchStatus {
	// Fetch remote info silently (ignore errors - offline is fine)
	_, _ = runGit(repoPath, "fetch", "--all", "--quiet")

	branches := make(map[string]*BranchStatus)

	// Local branches
	out, err := runGit(repoPath, "for-each-ref", "--format=%(refname:short) %(upstream:short) %(upstream:track)", "refs/heads/")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" {
				continue
			}
			parts := strings.Fields(line)
			name := parts[0]
			bs := &BranchStatus{
				Name:    name,
				IsLocal: true,
			}
			// Check if has upstream
			if len(parts) >= 2 && parts[1] != "" {
				bs.IsRemote = true
				// Parse tracking info like [ahead 2, behind 1]
				trackInfo := strings.Join(parts[2:], " ")
				bs.Ahead, bs.Behind = parseTrackInfo(trackInfo)
			}
			branches[name] = bs
		}
	}

	// Remote-only branches (not yet checked out locally)
	out, err = runGit(repoPath, "for-each-ref", "--format=%(refname:short)", "refs/remotes/origin/")
	if err == nil {
		for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
			if line == "" || line == "origin/HEAD" {
				continue
			}
			name := strings.TrimPrefix(line, "origin/")
			if _, exists := branches[name]; !exists {
				branches[name] = &BranchStatus{
					Name:     name,
					IsLocal:  false,
					IsRemote: true,
				}
			}
		}
	}

	// Mark current branch
	currentOut, err := runGit(repoPath, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		current := strings.TrimSpace(currentOut)
		if bs, ok := branches[current]; ok {
			bs.IsCurrent = true
		}
	}

	var result []BranchStatus
	for _, bs := range branches {
		result = append(result, *bs)
	}
	return result
}

// parseTrackInfo parses git tracking info like "[ahead 2, behind 1]" or "[ahead 2]".
func parseTrackInfo(info string) (ahead, behind int) {
	info = strings.Trim(info, "[]")
	if info == "" {
		return 0, 0
	}
	parts := strings.Split(info, ",")
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if strings.HasPrefix(p, "ahead ") {
			ahead, _ = strconv.Atoi(strings.TrimPrefix(p, "ahead "))
		}
		if strings.HasPrefix(p, "behind ") {
			behind, _ = strconv.Atoi(strings.TrimPrefix(p, "behind "))
		}
	}
	return
}
