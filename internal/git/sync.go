package git

import (
	"fmt"
)

// SyncResult holds the result of a sync operation.
type SyncResult struct {
	Action  string // "push", "pull", "blocked", "noop"
	Message string
	Err     error
}

// SyncRepo attempts to sync the current branch of a repo.
// It follows conservative sync rules:
// - Only push if ahead and not behind
// - Only pull if behind and not ahead
// - Block if diverged (both ahead and behind)
// - Block if there are uncommitted changes
func SyncRepo(repoPath string) SyncResult {
	// Fetch latest remote state before syncing
	FetchRemote(repoPath)
	status := GetRepoStatus(repoPath)

	if !status.IsGitRepo {
		return SyncResult{Action: "blocked", Message: "Not a git repository"}
	}
	if !status.HasRemote {
		return SyncResult{Action: "blocked", Message: "No remote configured"}
	}
	if status.Staged > 0 || status.Unstaged > 0 {
		return SyncResult{Action: "blocked", Message: "Uncommitted changes - commit first"}
	}

	// Find current branch status
	var current *BranchStatus
	for _, b := range status.Branches {
		if b.IsCurrent {
			bc := b
			current = &bc
			break
		}
	}
	if current == nil {
		return SyncResult{Action: "blocked", Message: "Could not determine current branch status"}
	}

	if current.Ahead > 0 && current.Behind > 0 {
		return SyncResult{
			Action:  "blocked",
			Message: fmt.Sprintf("Branch %s is diverged (↑%d ↓%d) - resolve manually", current.Name, current.Ahead, current.Behind),
		}
	}

	if current.Ahead > 0 {
		_, err := runGit(repoPath, "push", "origin", current.Name)
		if err != nil {
			return SyncResult{Action: "push", Message: "Push failed", Err: err}
		}
		return SyncResult{Action: "push", Message: fmt.Sprintf("Pushed %d commit(s) on %s", current.Ahead, current.Name)}
	}

	if current.Behind > 0 {
		_, err := runGit(repoPath, "pull", "--ff-only", "origin", current.Name)
		if err != nil {
			// Roll back on merge issues
			_, _ = runGit(repoPath, "merge", "--abort")
			return SyncResult{Action: "pull", Message: "Pull failed (rolled back)", Err: err}
		}
		return SyncResult{Action: "pull", Message: fmt.Sprintf("Pulled %d commit(s) on %s", current.Behind, current.Name)}
	}

	return SyncResult{Action: "noop", Message: "Already synced"}
}

// CloneRepo clones a repository to the given path.
func CloneRepo(remoteURL, targetPath string) error {
	_, err := runGit(".", "clone", remoteURL, targetPath)
	if err != nil {
		return fmt.Errorf("clone failed: %v", err)
	}
	return nil
}
