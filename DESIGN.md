# Projects Sync Manager (PSM) - Design Document

## Overview

A terminal UI (TUI) application for managing and syncing multiple Git repositories across machines. It provides a split-panel interface showing folder structure (left) and git status/actions (right), with a reference file system for cross-machine synchronization.

## Distribution & Invocation

- **Language**: Go (single static binary, no runtime dependencies)
- **Binary compression**: UPX-compressed release binaries (~5-7 MB)
- **Installation options**:
  - Direct binary download from GitHub releases
  - `go install github.com/<user>/projects-sync-manager@latest`
- **Invocation**: `psm [-d <directory>] [-h <depth>]`

### CLI Flags

| Flag | Description |
|------|-------------|
| `-d <path>` | Target directory to scan (default: current directory) |
| `-h <depth>` | Max depth for initial git repo discovery (default: 3) |

---

## UI Layout

```
+--------------------------------------+--------------------------------------+
|           LEFT PANEL                 |           RIGHT PANEL                |
|         (Navigation)                 |         (Information)                |
|                                      |                                      |
|  projects/                           |  Repository: my-project              |
|  +-- 01_Web/                         |  Remote: git@github.com:user/repo    |
|  |   +-- my-project/    ● ✓ ↑0 ↓0    |  URL: https://github.com/user/repo   |
|  |   +-- another-proj/  ● △ ↑2 ↓0   |                                       |
|  |   +-- static-site/   ○            |  === Branch Status ===               |
|  +-- 02_Tooling/                     |  main        : ✓ synced              |
|  |   +-- sync-mgr/      ● ? ↑0 ↓0    |  feature/foo : ↑2 ↓1 diverged        |
|  +-- 03_Data/                        |  dev         : local only            |
|  |   +-- scripts/        ○           |  hotfix/bar  : △ uncommitted         |
|      (no git repos)                  |                                      |
|                                      |  === Current Branch: main ===        |
|                                      |  Staged: 1 file                      |
|                                      |  Unstaged: 2 files                   |
|                                      |  Untracked: 3 files                  |
|                                      |                                      |
|                                      +--------------------------------------+
|                                      |           ACTIONS                    |
|                                      |  [S] Sync (pull/push)                |
|                                      |  [C] Open in VS Code                 |
|                                      |  [E] Open in File Explorer           |
|                                      |  [B] Open in Browser                 |
+--------------------------------------+--------------------------------------+
|                         STATUS BAR / COMMANDS                               |
|  [R] Refresh  [F] Reference File  [Q] Quit  [?] Help                        |
+-----------------------------------------------------------------------------+
```

---

## Left Panel - Navigation

### Tree Display Rules

1. **Git repositories** are always shown in the tree with status indicators
2. **Folders containing git repos** (at any depth) are shown expanded
3. **Folders with NO git repos inside**: shown as a single collapsed node (depth 1 only - just the folder name). Expands on right-arrow navigation to reveal contents
4. Tree format mimics the `tree` command output (with `+--` connectors)
5. **Folder names wrap** if they exceed the panel width (long names don't get clipped)

### Status Indicators (inline, after folder name)

Each git repo folder shows a compact status line composed of multiple indicators:

```
folder-name/   ● ✓ ↑0 ↓3
               │ │ │   └─ 3 commits to pull
               │ │ └───── 0 commits to push
               │ └─────── sync status icon
               └───────── git indicator (● = git repo)
```

#### 1. Git Indicator

| Symbol | Meaning |
|--------|---------|
| `●` | This is a git repository |
| `○` | Not a git repository (entire line shown in grey) |

#### 2. Folder Color

| Color | Meaning |
|-------|---------|
| **Grey** | Not a git repo |
| **Blue** | Git repo, no remote configured |
| **Green** | Git repo, remote configured, fully synced |
| **Yellow** | Git repo, partially synced (some branches ahead/behind) |
| **Red** | Git repo, has uncommitted changes or diverged branches |

#### 3. Sync Status Icon

| Symbol | Color | Meaning |
|--------|-------|---------|
| `✓` | Green | All branches synced with remote |
| `△` | Yellow | Partially synced (some branches ahead/behind) |
| `✗` | Red | Has uncommitted/unstaged changes |
| `?` | Blue | No remote repository configured |
| _(none)_ | — | Not a git repo |

#### 4. Push/Pull Counters

| Indicator | Meaning |
|-----------|---------|
| `↑N` | N commits ahead (to push) across all branches |
| `↓N` | N commits behind (to pull) across all branches |

Only shown for git repos with a remote. Omitted if both are 0 and repo is fully synced.

#### 5. Changes Indicator

| Symbol | Meaning |
|--------|---------|
| `+S` | S staged changes ready to commit |
| `~U` | U unstaged modifications |
| `…T` | T untracked files |

Only shown when there are pending changes. Appended after push/pull counters.

#### Full Examples

```
my-project/        ● ✓                      # green, fully synced, clean
api-server/        ● △ ↑2 ↓0               # yellow, 2 commits to push
webapp/            ● ✗ ↑0 ↓3 +1 ~4 …2      # red, 3 to pull, 1 staged, 4 unstaged, 2 untracked
experiments/       ● ?                       # blue, no remote
random-folder/     ○                         # grey, not a git repo
```

### Navigation Keys

| Key | Action |
|-----|--------|
| `Up/Down` | Navigate between items at the same directory level |
| `Right` | Enter/expand directory |
| `Left` | Go back to parent directory / collapse |
| `Enter` | Select (context-dependent) |

---

## Right Panel - Information

When a git repository is selected, shows:

### Top Section - Repository Info
- Repository name
- Remote URL (original)
- HTTPS URL (auto-converted from SSH if needed: `git@github.com:user/repo.git` -> `https://github.com/user/repo`)
- Current branch

### Middle Section - Branch Status
For **every** branch (local + remote-tracking):
- Branch name
- Sync status:
  - `✓ synced` - local and remote are identical
  - `↑X ahead` - local has commits not pushed
  - `↓X behind` - remote has commits not pulled
  - `↑X ↓Y diverged` - diverged
  - `local only` - branch exists locally but not on remote
  - `remote only` - branch exists on remote but not locally
  - `△ uncommitted` - has dirty working tree (only for current branch)

### Bottom Section - Actions

| Key | Action | Condition |
|-----|--------|-----------|
| `S` | Sync (pull or push) | Only when already committed. See sync rules below |
| `C` | Open in VS Code | Runs `code <path>` |
| `E` | Open in File Explorer | OS-aware: `explorer.exe .` (WSL/Windows), `xdg-open .` (Linux), `open .` (macOS) |
| `B` | Open remote in browser | Opens HTTPS remote URL in default browser |

---

## Sync Rules (Critical Safety Logic)

Sync is **intentionally conservative**:

| Local State | Remote State | Sync Action |
|-------------|-------------|-------------|
| Uncommitted changes | Any | **BLOCKED** - must commit first |
| Ahead (has pushable commits) | Up to date | Auto **push** |
| Up to date | Ahead (has pullable commits) | Auto **pull** |
| Ahead | Also ahead (diverged) | **BLOCKED** - manual resolution needed |
| Synced | Synced | No-op |

- Sync operates on the **current branch only** when triggered per-repo
- Bulk sync (from reference file view) applies the same rules per-branch per-repo
- **Never** auto-resolves merge conflicts - rolls back on conflict
- **Never** does pull+push together in one operation if both are needed
- **After sync completes**, left panel status indicators auto-refresh for the affected repo

---

## Reference File System

### Purpose
Portable snapshot of your project structure for cross-machine comparison. Contains **only** folder paths and git remote URLs - no status information.

### File Format (`projects-ref.json`)

```json
{
  "version": 1,
  "created_at": "2025-01-15T10:30:00Z",
  "base_path": "~/projects",
  "repositories": [
    {
      "relative_path": "01_Web/my-project",
      "remote_url": "https://github.com/user/my-project.git"
    },
    {
      "relative_path": "02_Tooling/sync-manager",
      "remote_url": "https://github.com/user/sync-manager.git"
    }
  ]
}
```

### Reference File Operations

| Action | Description |
|--------|-------------|
| **Generate** | Scan current directory, create reference file from all discovered git repos |
| **Load/Compare** | Load a reference file and compare against current directory state |

### Comparison View (when reference file is loaded)

The UI stays as a split panel but changes to comparison mode:

**Left Panel indicators**:

| Indicator | Meaning |
|-----------|---------|
| `[++]` (green) | Exists locally but NOT in reference file (extra repo) |
| `[--]` (red) | In reference file but NOT found locally (missing repo) |
| `[==]` | Present in both, matched |

**Right Panel** (for missing repos):
- Shows the remote URL from the reference file
- **Clone** action: clones the repo into the expected relative path
- **Clone All Missing**: bulk action to clone all `[--]` repos at once

**Right Panel** (for existing repos):
- Normal git status info (same as default view)
- **Sync All**: bulk sync all repos that are safe to sync (already committed, not diverged)

---

## Global Commands

| Key | Action |
|-----|--------|
| `R` | Refresh - rescan all repos and update status |
| `F` | Reference file menu (generate / load / compare) |
| `Q` | Quit |
| `?` | Help overlay |

---

## Technical Approach

### Performance

- **Multi-threaded scanning**: Git status checks run concurrently using goroutines with a worker pool (bounded to avoid spawning 60+ git processes at once). Each repo's status is fetched in parallel during initial scan and refresh.
- **Async initial load**: Show the TUI immediately with a loading spinner; populate repo statuses as they come in.
- **Auto-refresh after actions**: After sync/pull/push operations, only the affected repo is re-scanned (not the entire tree).

### Responsive Terminal

- The TUI listens for terminal resize events (`tea.WindowSizeMsg` in bubbletea)
- On resize: recalculates panel widths, re-wraps folder names, and redraws the full screen
- Left/right panel split adapts to terminal width (roughly 40%/60%, minimum widths enforced)
- Folder names wrap within the left panel rather than being truncated

### Build & Release

- **Binary compression**: Release binaries are compressed with [UPX](https://upx.github.io/) to reduce size (~5-7 MB final)
- **Cross-compilation**: Built for `linux/amd64`, `linux/arm64`, `darwin/amd64`, `darwin/arm64`, `windows/amd64` via `GOOS`/`GOARCH`
- **Release automation**: Git tags trigger GitHub Actions to build, compress, and upload release binaries

### Go Libraries
- **TUI**: [bubbletea](https://github.com/charmbracelet/bubbletea) + [lipgloss](https://github.com/charmbracelet/lipgloss) (Charm stack)
- **Git operations**: shell out to `git` CLI (avoids libgit2 dependency, works everywhere)
- **File format**: JSON for reference files

### Architecture

```
cmd/
  main.go              # entry point, flag parsing
internal/
  git/
    repo.go            # git repo detection, status queries
    branch.go          # branch listing, sync status
    sync.go            # pull/push/clone operations
    remote.go          # remote URL parsing, SSH->HTTPS conversion
  scanner/
    scanner.go         # directory tree scanning with goroutine pool
  reference/
    reference.go       # generate/load/compare reference files
  network/
    protocol.go        # WebSocket message types, PeerTree, conversions
    peer.go            # peer connection wrapper (read loop → channel)
    server.go          # HTTP + WebSocket server with code auth
    client.go          # WebSocket client dialer + auth handshake
  tui/
    app.go             # bubbletea main model, resize handling
    tree.go            # left panel - tree navigation
    info.go            # right panel - repo info display
    actions.go         # right panel - action handlers
    compare.go         # reference file comparison view
    network.go         # peer sync views, remote actions, virtual peer tree
    styles.go          # lipgloss styles and colors
  opener/
    opener.go          # OS-aware open commands (vscode, explorer, browser)
```

---

## Peer-to-Peer Sync

### Connection Flow

1. **Server**: User presses `N` → "Start Server" → enters port (default 3000) → server starts, generates 4-char code, displays local IPs
2. **Client**: User presses `N` → "Connect" → enters URL + code → connects via WebSocket
3. **Handshake**: Client sends code + hostname, server validates → both exchange repo trees
4. **Live comparison**: Both see comparison views with 5 tabs. Changes auto-propagate

### Protocol (JSON over WebSocket)

```
Client → Server:  {"type":"auth","data":{"code":"A7K2","hostname":"my-pc"}}
Server → Client:  {"type":"auth_ok","data":{"hostname":"server-pc"}}
                   {"type":"auth_fail"}
Bidirectional:     {"type":"tree","data":{PeerTree}}
                   {"type":"clone_req","data":{"path":"...","url":"..."}}
                   {"type":"sync_req","data":{"path":"..."}}
                   {"type":"action_res","data":{"action":"clone","path":"...","message":"..."}}
                   {"type":"disconnect"}
```

### Five Views

| Tab | View | Actions |
|-----|------|---------|
| 1 | Combined — all repos from both | Red=clone local, Yellow=clone on peer |
| 2 | Local perspective — peer as reference | Actions on this machine |
| 3 | Remote perspective — inverted | Actions on peer |
| 4 | My Tree — normal local view | All normal actions |
| 5 | Peer Tree — peer's repos | Remote sync via peer |

### Remote Actions

When a peer requests an action (clone/sync), the receiving side:
1. Executes the action locally
2. Sends back an `action_res` with the result
3. Sends an updated tree so the requester's view refreshes

---

## Open Questions

1. **Multiple remotes**: If a repo has multiple remotes (origin, upstream), which to display/sync with? Suggestion: default to `origin`, show all in info panel.
2. **Submodules**: Should git submodules be treated as separate repos or ignored?
3. **Reference file location**: Store in the scanned directory root, or in a config directory (`~/.config/projects-sync-manager/`)?
4. **Branch sync scope**: When bulk syncing, sync all branches or only the currently checked-out branch per repo?
5. **Stash handling**: If there are uncommitted changes and user wants to pull, should we offer to stash-pull-unstash?
