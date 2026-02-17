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

// SyncBranch syncs a specific branch without switching to it.
// For the current branch, it uses pull/push normally.
// For non-current branches, it uses fetch + branch -f (pull) or push origin <branch> (push).
func SyncBranch(repoPath string, branch BranchStatus) SyncResult {
	FetchRemote(repoPath)

	// Re-read branch status after fetch
	branches := GetBranchStatuses(repoPath)
	var b *BranchStatus
	for i := range branches {
		if branches[i].Name == branch.Name {
			b = &branches[i]
			break
		}
	}
	if b == nil {
		return SyncResult{Action: "blocked", Message: fmt.Sprintf("Branch %s not found", branch.Name)}
	}

	if !b.IsRemote {
		return SyncResult{Action: "blocked", Message: fmt.Sprintf("Branch %s has no remote tracking", b.Name)}
	}

	if b.Ahead > 0 && b.Behind > 0 {
		return SyncResult{
			Action:  "blocked",
			Message: fmt.Sprintf("Branch %s is diverged (↑%d ↓%d) - resolve manually", b.Name, b.Ahead, b.Behind),
		}
	}

	if b.Ahead > 0 {
		// Push works for any branch without switching
		_, err := runGit(repoPath, "push", "origin", b.Name)
		if err != nil {
			return SyncResult{Action: "push", Message: "Push failed", Err: err}
		}
		return SyncResult{Action: "push", Message: fmt.Sprintf("Pushed %d commit(s) on %s", b.Ahead, b.Name)}
	}

	if b.Behind > 0 {
		if b.IsCurrent {
			// Current branch: use pull --ff-only
			_, err := runGit(repoPath, "pull", "--ff-only", "origin", b.Name)
			if err != nil {
				_, _ = runGit(repoPath, "merge", "--abort")
				return SyncResult{Action: "pull", Message: "Pull failed (rolled back)", Err: err}
			}
		} else {
			// Non-current branch: fast-forward the local ref without switching
			// git fetch origin <branch>:<branch> does a fast-forward update
			_, err := runGit(repoPath, "fetch", "origin", b.Name+":"+b.Name)
			if err != nil {
				return SyncResult{Action: "pull", Message: fmt.Sprintf("Pull failed for %s (may not be fast-forward)", b.Name), Err: err}
			}
		}
		return SyncResult{Action: "pull", Message: fmt.Sprintf("Pulled %d commit(s) on %s", b.Behind, b.Name)}
	}

	return SyncResult{Action: "noop", Message: fmt.Sprintf("Branch %s is already synced", b.Name)}
}

// CloneRepo clones a repository to the given path.
func CloneRepo(remoteURL, targetPath string) error {
	_, err := runGit(".", "clone", remoteURL, targetPath)
	if err != nil {
		return fmt.Errorf("clone failed: %v", err)
	}
	return nil
}
