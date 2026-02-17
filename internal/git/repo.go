package git

import (
	"os"
	"path/filepath"
	"strings"
)

// RepoStatus holds the full status of a git repository.
type RepoStatus struct {
	Path          string
	IsGitRepo     bool
	HasRemote     bool
	RemoteURL     string
	HTTPSURL      string
	CurrentBranch string
	Branches      []BranchStatus
	Staged        int
	Unstaged      int
	Untracked     int
	TotalAhead    int // sum across all branches
	TotalBehind   int // sum across all branches
}

// SyncState returns the overall sync state for display coloring.
func (r *RepoStatus) SyncState() SyncState {
	if !r.IsGitRepo {
		return StateNotGit
	}
	if !r.HasRemote {
		return StateNoRemote
	}
	if r.Staged > 0 || r.Unstaged > 0 || r.Untracked > 0 {
		return StateDirty
	}
	for _, b := range r.Branches {
		if b.Ahead > 0 && b.Behind > 0 {
			return StateDirty
		}
	}
	if r.TotalAhead > 0 || r.TotalBehind > 0 {
		return StatePartial
	}
	return StateSynced
}

// SyncState represents the overall sync state of a repo.
type SyncState int

const (
	StateNotGit   SyncState = iota
	StateNoRemote           // git repo but no remote
	StateSynced             // all branches synced
	StatePartial            // some branches ahead/behind
	StateDirty              // uncommitted changes or diverged
)

// IsGitRepository checks if a directory is a git repository.
func IsGitRepository(path string) bool {
	gitDir := filepath.Join(path, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// GetRepoStatus returns the full status of a git repository.
func GetRepoStatus(path string) *RepoStatus {
	status := &RepoStatus{
		Path:      path,
		IsGitRepo: IsGitRepository(path),
	}
	if !status.IsGitRepo {
		return status
	}

	// Current branch
	branch, err := runGit(path, "rev-parse", "--abbrev-ref", "HEAD")
	if err == nil {
		status.CurrentBranch = strings.TrimSpace(branch)
	}

	// Remote
	status.RemoteURL = GetOriginURL(path)
	status.HasRemote = status.RemoteURL != ""
	if status.HasRemote {
		status.HTTPSURL = SSHToHTTPS(status.RemoteURL)
	}

	// Working tree status
	status.Staged, status.Unstaged, status.Untracked = getWorkingTreeStatus(path)

	// Branch statuses
	if status.HasRemote {
		status.Branches = GetBranchStatuses(path)
		for _, b := range status.Branches {
			status.TotalAhead += b.Ahead
			status.TotalBehind += b.Behind
		}
	}

	return status
}

// getWorkingTreeStatus returns counts of staged, unstaged, and untracked files.
func getWorkingTreeStatus(path string) (staged, unstaged, untracked int) {
	out, err := runGit(path, "status", "--porcelain")
	if err != nil {
		return 0, 0, 0
	}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if len(line) < 2 {
			continue
		}
		x := line[0]
		y := line[1]
		if x == '?' {
			untracked++
		} else {
			if x != ' ' && x != '?' {
				staged++
			}
			if y != ' ' && y != '?' {
				unstaged++
			}
		}
	}
	return
}
