# CLAUDE.md — Projects Sync Manager (PSM)

## Project Overview
A Go-based TUI application for managing and syncing 60+ git repositories across multiple machines. Single static binary, no runtime dependencies.

## Tech Stack
- **Language:** Go (module: `github.com/sid/psm`)
- **TUI:** bubbletea + lipgloss (Charm stack)
- **Git:** shells out to `git` CLI with `-c safe.directory=*` on all commands
- **Build:** `CGO_ENABLED=0` for static binaries, UPX compression for linux releases

## Architecture
```
cmd/main.go                    # entry point, -d/-h/--version flags
internal/
  git/
    repo.go                    # RepoStatus, SyncState, GetRepoStatus/GetRepoStatusFresh
    branch.go                  # BranchStatus, FetchRemote, GetBranchStatuses
    sync.go                    # SyncRepo, SyncBranch (per-branch without checkout), CloneRepo
    remote.go                  # runGit (all git commands go through here), SSHToHTTPS
  scanner/
    scanner.go                 # ScanDirectory, TreeNode, ExpandNode, RefreshNode, RefreshAll
    ignore.go                  # .psmignore + built-in ignore patterns
  reference/
    reference.go               # Generate/Save/Load/Compare reference files (JSON)
  tui/
    app.go                     # Main bubbletea Model, all message handling, View()
    tree.go                    # Left panel tree rendering, legend, centeredWindow
    nav.go                     # Tree navigation (up/down siblings, left/right expand/collapse)
    info.go                    # Right panel info display (normal mode)
    detail.go                  # Interactive repo detail panel (branches + actions, Tab to switch)
    actions.go                 # Async action handlers (sync, open, refresh)
    compare.go                 # Reference file comparison view (tree-based)
    styles.go                  # All lipgloss colors and styles
  opener/
    opener.go                  # OS-aware openers (VSCode, explorer, browser) with WSL fallback
```

## Key Design Decisions
- **No git fetch on initial scan** — only local tracking refs are used. Fetch happens on explicit refresh or sync only.
- **Conservative sync** — never auto-resolves conflicts, never push+pull in same operation, blocks on uncommitted changes or diverged branches.
- **Per-branch sync without checkout** — uses `git push origin <branch>` and `git fetch origin <branch>:<branch>` for non-current branches.
- **Folders containing git repos cannot be collapsed** — `HasGitDescendant()` check in navigation.
- **Individual refresh (`r`)** — fetches from remote for just the selected repo. Full refresh (`R`) requires y/n confirmation.

## Building
```bash
# Dev build
CGO_ENABLED=0 go build -ldflags="-s -w" -o /tmp/psm ./cmd/

# Test against a directory
/tmp/psm -d ~/projects
```

## Creating a Release
Refer to the "Creating a GitHub Release" section in README.md. In short:
```bash
git tag -a v0.X.0 -m "Release description"
git push origin v0.X.0
```
The GitHub Actions workflow (`.github/workflows/release.yml`) automatically builds all platform binaries, creates source archives, generates SHA256 checksums, and publishes the release.

## Important Patterns
- All git commands go through `runGit()` in `internal/git/remote.go` which adds `-c safe.directory=*`
- Scanner uses a worker pool of 10 goroutines for concurrent status fetching
- Progress is reported via channels from scanner to TUI using `tea.Batch`
- Navigation is sibling-based (up/down move between siblings, not sequential list items)
- `var version = "dev"` in main.go is overridden at build time via `-ldflags`

## TODOs
See `TODO.md` for planned improvements (relocated repo detection, multiple remote handling, etc.)
