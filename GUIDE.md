# PSM User Guide

A comprehensive guide to using Projects Sync Manager.

---

## Table of Contents

- [Overview](#overview)
- [Installation](#installation)
- [Getting Started](#getting-started)
- [Navigation](#navigation)
- [Status Indicators](#status-indicators)
- [Syncing Repos](#syncing-repos)
- [Repo Detail Panel](#repo-detail-panel)
- [Refreshing](#refreshing)
- [Command Palette](#command-palette)
- [Filters](#filters)
- [Reference Files](#reference-files)
- [Peer-to-Peer Sync](#peer-to-peer-sync)
- [Opening External Tools](#opening-external-tools)
- [Ignore File](#ignore-file)
- [Tips & Workflows](#tips--workflows)

---

## Overview

PSM scans a directory tree for git repositories and presents them in a two-panel TUI. The left panel shows a navigable tree with color-coded sync indicators. The right panel shows details for the selected repo — branches, working tree status, and available actions.

Key design principles:
- **No automatic network calls.** PSM reads local git tracking refs on startup. Remote fetches only happen when you explicitly refresh.
- **Conservative sync.** Never auto-resolves conflicts. Blocks on uncommitted changes or diverged branches. Uses `--ff-only` for pulls.
- **Single static binary.** No runtime dependencies beyond `git`.

---

## Installation

### Quick Start (recommended)

**Linux / macOS:**
```bash
curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.sh | sh
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.ps1 | iex
```

The script downloads the correct binary for your OS/architecture, caches it, and runs it. Subsequent runs reuse the cached binary.

### For Regular Use

Download the script locally so you can run it without piping from the internet each time:

```bash
# Download once
curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/psm.sh -o ~/.local/bin/psm.sh && chmod +x ~/.local/bin/psm.sh

# Run anytime
psm.sh -d ~/projects
```

The script auto-updates — it checks for new releases on each run.

### Install to PATH

```bash
sudo curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-amd64 \
  -o /usr/local/bin/psm && sudo chmod +x /usr/local/bin/psm
```

### Build from Source

```bash
go build -ldflags="-s -w" -o psm ./cmd/
```

---

## Getting Started

```bash
# Scan current directory (default depth: 3)
psm

# Scan a specific directory
psm -d ~/projects

# Scan deeper
psm -d ~/projects -h 5
```

On first run, PSM creates a `.psmignore` file with sensible defaults. The initial scan only reads local git state — no network calls are made.

---

## Navigation

PSM uses **sibling-based navigation**. Up/Down move between items at the same directory level, not through a flat list.

| Key | Action |
|-----|--------|
| `↑` or `k` | Previous sibling |
| `↓` or `j` | Next sibling |
| `→` or `l` | Enter / expand directory |
| `←` or `h` | Collapse directory / go to parent |
| `Enter` | Open detail panel for a git repo |

**Important:** Directories containing git repos cannot be collapsed. This ensures you always see your repos.

To navigate into a subdirectory's children, press `→` to expand it, then `→` again to enter it. To go back up, press `←`.

---

## Status Indicators

The tree view shows colored indicators next to each repo:

```
my-project/     ● ✓                    # synced (green)
api-server/     ● △ ↑2                 # 2 commits to push (yellow)
webapp/         ● ✗ ~4 …2              # uncommitted changes (red)
experiments/    ● ?                     # no remote configured (blue)
data-scripts/   ● ✓ ≠                  # name mismatch (yellow ≠)
some-folder/    ○                       # not a git repo (grey)
```

### Symbol Reference

| Symbol | Meaning |
|--------|---------|
| `●` | Git repository |
| `○` | Not a git repo |
| `✓` | All branches synced |
| `△` | Some branches ahead/behind |
| `✗` | Dirty (uncommitted changes or diverged branches) |
| `?` | No remote configured |
| `≠` | Folder name differs from remote repo name |
| `↑N` | N commits to push |
| `↓N` | N commits to pull |
| `+N` | N staged files |
| `~N` | N unstaged files |
| `…N` | N untracked files |

### Name Mismatch (`≠`)

The `≠` indicator appears when your local folder name doesn't match the repository name from the remote URL. For example, if you cloned `github.com/user/my-app` into a folder called `app/`, the `≠` will show. This helps identify repos that may have been renamed or cloned into non-standard directories.

### Renaming to Match (`n`)

Press `n` on a repo with the `≠` indicator to rename the folder to match the remote repo name. PSM will show a confirmation prompt before renaming. You can also use this from the detail panel (`Enter` on the repo, then select "Rename folder to ...").

---

## Syncing Repos

Press `s` on a git repo to sync its current branch:

- **Ahead only:** Pushes to remote
- **Behind only:** Pulls with `--ff-only` (fast-forward only, no merge commits)
- **Diverged:** Blocked — you need to resolve manually
- **Uncommitted changes:** Blocked — commit or stash first
- **Up to date:** No action needed

The right panel shows what will happen before you press sync:
```
[s] Sync (push 2 commits)
[s] Sync (pull 3 commits)
[s] Sync (blocked: uncommitted changes)
[s] Sync (up to date)
```

### Per-Branch Sync (Detail Panel)

For more control, press `Enter` to open the detail panel. You can select any branch and sync it individually — even non-current branches are synced without checkout using `git fetch origin branch:branch` and `git push origin branch`.

---

## Repo Detail Panel

Press `Enter` on a git repo to open the interactive detail panel.

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate within the focused section |
| `Tab` | Switch between Branches and Actions |
| `Enter` | Select a branch / execute an action |
| `Esc` | Close the detail panel |

### Branches Section

Lists all local and remote branches with their sync status. The current branch is marked with `*`. Select a branch to see its available actions.

### Actions Section

Context-sensitive actions based on the selected branch:
- **Refresh this repo** — fetch from remote and update status
- **Sync branch: \<name\>** — push/pull the selected branch
- **Sync current branch** — same as pressing `s` in the tree
- **Open in VS Code / Explorer / Browser** — external tool integrations

---

## Refreshing

| Key | Action |
|-----|--------|
| `r` | Refresh the selected repo (fetches from remote) |
| `R` | Refresh ALL repos (requires `y` confirmation) |

The initial scan only reads local refs — no network calls. Press `r` on a repo to fetch from its remote and update the displayed status. Press `R` to fetch all repos (this can take a while with many repos).

---

## Command Palette

Press `/` to open the command palette — a VS Code-style fuzzy search bar for all available commands.

### Using the Palette

| Key | Action |
|-----|--------|
| Type | Filter commands by fuzzy search |
| `↑` / `↓` | Navigate results |
| `Tab` | Auto-complete with selected command name |
| `Enter` | Execute the selected command |
| `Esc` | Close the palette |

### Available Commands

| Command | Description |
|---------|-------------|
| Sync | Sync the selected repo |
| Refresh | Refresh the selected repo |
| Refresh All | Refresh all repos |
| Open in VS Code | Open selected in VS Code |
| Open in Explorer | Open in file manager |
| Open in Browser | Open remote URL in browser |
| Reference: Generate | Generate a reference file |
| Reference: Load & Compare | Load and compare a reference file |
| Filter: Toggle Dirty | Toggle the dirty filter |
| Filter: Toggle Partial | Toggle the partial filter |
| Filter: Toggle Synced | Toggle the synced filter |
| Filter: Toggle No Remote | Toggle the no-remote filter |
| Filter: Toggle Ahead | Toggle the ahead filter |
| Filter: Toggle Behind | Toggle the behind filter |
| Filter: Toggle Name Mismatch | Toggle the name mismatch filter |
| Filter: Clear All | Clear all active filters |
| Filter: Open Panel | Open the filter panel |
| Network: Start Server | Start a peer sync server |
| Network: Connect to Peer | Connect to a peer sync server |
| Network: Disconnect | Disconnect from peer |
| Help | Show the help overlay |
| Quit | Quit the application |

### Fuzzy Search

The search matches characters in order, not necessarily consecutive. Typing `fd` matches **F**ilter: Toggle **D**irty. Word boundary matches (start of words after spaces or colons) score higher for more relevant results.

---

## Filters

Filters let you focus on repos that need attention. Multiple filters combine with OR logic — enabling "Dirty" and "Partial" shows repos that are dirty OR partially synced.

### Filter Panel

Press `F` (capital F) to open the filter panel overlay.

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate filters |
| `Space` or `Enter` | Toggle the selected filter |
| `c` | Clear all filters |
| `Esc` | Close the panel |

The panel shows checkboxes for each filter:
```
[ ] Dirty              — uncommitted changes or diverged
[x] Partial            — branches ahead/behind
[ ] Synced             — all branches synced
[ ] No Remote          — no remote configured
[ ] Ahead              — has commits to push
[ ] Behind             — has commits to pull
[ ] Name Mismatch      — folder name differs from repo name
```

### Active Filter Indicator

When filters are active:
- A blue status line appears below the tree legend showing active filters and match count: `Filter: dirty, partial (12 repos)`
- The status bar shows a yellow badge next to `Filter` with the active count: `Filter(2)`
- The tree only shows matching repos and their parent directories

### Quick Filter via Command Palette

Press `/` and type `filter dirty` to toggle the dirty filter without opening the panel. Or type `filter clear` to clear all filters.

---

## Reference Files

Reference files capture a snapshot of your project structure — which repos exist and their remote URLs. Use them to replicate your setup on another machine.

### How Matching Works

Comparison is **URL-based**, not path-based. PSM identifies repos by their remote URL (normalized to HTTPS, case-insensitive). This means:

- If you moved a repo from `01_Web/my-app/` to `02_Tools/my-app/`, PSM recognizes it as the same repo (same remote URL) and shows it as **relocated** rather than missing + extra.
- If you renamed a folder but kept the same remote, it's still matched.
- Folder names and paths are display information — the remote URL is the identity.

### Generate

Press `f` then `g` (or use the command palette: `Reference: Generate`).

This creates `projects-ref.json` in the scanned directory root. The file contains relative paths and HTTPS-normalized remote URLs for every git repo with a remote.

### Load & Compare

Press `f` then `l` (or use the command palette: `Reference: Load & Compare`).

PSM loads the reference file and compares it against the current directory tree:

| Status | Symbol | Color | Meaning |
|--------|--------|-------|---------|
| Matched | `[==]` | Green | Repo found locally at the expected path |
| Missing | `[--]` | Red | Repo in reference but not found locally |
| Extra | `[++]` | Yellow | Repo exists locally but not in reference |
| Relocated | `[⇄]` | Purple | Repo found locally but at a different path |

For **relocated** repos, the right panel shows both the expected path (from reference) and the actual path (where it was found).

### Compare View Navigation

| Key | Action |
|-----|--------|
| `↑` / `↓` | Navigate the comparison tree |
| `→` / `←` | Expand / collapse directories |
| `Enter` | Clone a missing repo |
| `a` | Clone ALL missing repos |
| `S` | Sync all matched repos |
| `Esc` | Return to normal view |

### Clone Protocol

When cloning, PSM checks if SSH access is available:
- If SSH works, you're prompted to choose SSH or HTTPS
- Press `A` to use SSH for all clones in the session
- Press `Y` for SSH just this once
- Press `H` for HTTPS
- If SSH is unavailable, HTTPS is used automatically

---

## Peer-to-Peer Sync

Compare and sync repos between two machines in real-time over WebSocket. No reference files needed — both PSM instances exchange their repo trees live.

### Starting a Connection

**Machine A (server):**
1. Press `N` to open the peer sync menu
2. Press `S` to start a server
3. Enter a port (default 3000), press `Enter`
4. Note the 4-character code and IP addresses shown

**Machine B (client):**
1. Press `N` to open the peer sync menu
2. Press `C` to connect
3. Enter the server's IP:port (e.g. `192.168.1.5:3000`)
4. Tab to the code field, enter the 4-character code
5. Press `Enter`

Both machines immediately enter the comparison view. If the code is wrong, you can re-enter it without restarting.

### Five Views

Once connected, use number keys `1`-`5` to switch between views:

| Tab | View | Description | Actions Execute On |
|-----|------|-------------|--------------------|
| `1` | Combined | All repos from both machines. Green=on both, Red=only on peer, Yellow=only on you | Red→clone locally, Yellow→clone on peer |
| `2` | Local | Peer's tree as reference, compared against your local repos | This machine |
| `3` | Remote | Your tree as reference, from peer's perspective | Peer's machine |
| `4` | My Tree | Normal tree view of your local repos | This machine |
| `5` | Peer Tree | Normal tree view of peer's repos with their sync states | Peer's machine |

### Compare View Legend

| Symbol | Color | Meaning |
|--------|-------|---------|
| `[==]` | Green | Repo exists on both machines at same relative path |
| `[--]` | Red | Repo exists on the reference side but not locally |
| `[++]` | Yellow | Repo exists locally but not on the reference side |
| `[⇄]` | Purple | Same repo (by URL) but at a different relative path |

### Actions

In views 1-3 (compare views):
- `Enter` — Clone the selected missing repo (locally or on peer, depending on view)
- `a` — Clone all missing repos
- Navigation with arrow keys / hjkl

In view 4 (My Tree):
- All normal actions work: `s` sync, `r` refresh, `c` VS Code, `e` explorer, `b` browser, `Enter` detail panel

In view 5 (Peer Tree):
- `s` — Request the peer to sync the selected repo
- Navigation with arrow keys / hjkl

### Live Updates

Changes propagate automatically. When you clone or sync a repo, your tree is re-sent to the peer, and their comparison view updates in real-time. The same happens when the peer makes changes.

### Disconnecting

- Press `D` to disconnect from the peer
- Press `Esc` to disconnect and return to normal view
- Press `q` to quit (also disconnects)

### Peer Sync Keyboard Reference

| Key | Action |
|-----|--------|
| `N` | Open peer sync menu |
| `1`-`5` | Switch between views |
| `Enter` | Clone missing repo (views 1-3) |
| `a` | Clone all missing repos (views 1-3) |
| `s` | Sync (view 4: local, view 5: on peer) |
| `D` | Disconnect |
| `Esc` | Disconnect and return to normal |

---

## Opening External Tools

| Key | Action |
|-----|--------|
| `c` | Open in VS Code |
| `e` | Open in file explorer |
| `b` | Open remote URL in browser |

These work on both regular directories and git repos. On WSL, PSM automatically converts paths and uses Windows-side applications.

---

## Ignore File

PSM auto-generates a `.psmignore` file on first run with defaults covering:
- Hidden directories (`.git`, `.cache`, etc.)
- Build outputs (`build`, `dist`, `target`, `bin`, `obj`)
- Dependencies (`node_modules`, `vendor`, `__pycache__`)
- IDE directories (`.idea`, `.vscode`)
- Framework caches (`.next`, `.nuxt`)

Add custom regex patterns (one per line) to skip additional directories. Lines starting with `#` are comments.

---

## Tips & Workflows

### Morning Sync Routine

1. Launch PSM: `psm -d ~/projects`
2. Press `R` then `y` to refresh all repos from remotes
3. Press `F` to open filters, enable "Partial" to see what needs syncing
4. Navigate to each repo and press `s` to sync

### Setting Up a New Machine

1. Copy `projects-ref.json` from your existing machine
2. Create the target directory: `mkdir ~/projects`
3. Place the reference file: `cp projects-ref.json ~/projects/`
4. Run PSM: `psm -d ~/projects`
5. Press `f` then `l` to load the reference
6. Press `a` to clone all missing repos

### Finding Repos That Need Attention

Use filters to focus:
- **Dirty** — repos with uncommitted work
- **Ahead** — repos with unpushed commits
- **Behind** — repos that need pulling
- **Name Mismatch** — repos cloned into non-standard directories

### Keyboard Efficiency

- Use `/` (command palette) for any action — faster than remembering individual keys
- Use `F` for filter panel when you want to combine multiple filters visually
- Use `Enter` on a repo for the detail panel when you need per-branch control

---

## Key Reference

### Normal View

| Key | Action |
|-----|--------|
| `↑/k` `↓/j` | Navigate siblings |
| `→/l` `←/h` | Enter/exit directories |
| `Enter` | Open detail panel |
| `/` | Command palette |
| `F` | Filter panel |
| `s` | Sync selected repo |
| `r` | Refresh selected repo |
| `R` | Refresh all repos |
| `f` | Reference file menu |
| `c` | Open in VS Code |
| `e` | Open in file explorer |
| `b` | Open in browser |
| `n` | Rename folder to match repo name |
| `N` | Peer sync menu |
| `?` | Help |
| `q` | Quit |

### Detail Panel

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate |
| `Tab` | Switch section |
| `Enter` | Execute action |
| `Esc` | Close |

### Filter Panel

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate |
| `Space/Enter` | Toggle filter |
| `c` | Clear all |
| `Esc` | Close |

### Command Palette

| Key | Action |
|-----|--------|
| Type | Fuzzy search |
| `↑/↓` | Navigate |
| `Tab` | Auto-complete |
| `Enter` | Execute |
| `Esc` | Close |

### Compare View

| Key | Action |
|-----|--------|
| `↑/↓/←/→` | Navigate |
| `Enter` | Clone missing repo |
| `a` | Clone all missing |
| `S` | Sync all matched |
| `Esc` | Back to normal view |

### Peer Sync

| Key | Action |
|-----|--------|
| `1`-`5` | Switch views |
| `Enter` | Clone missing (views 1-3) |
| `a` | Clone all missing (views 1-3) |
| `s` | Sync (view 4: local, view 5: peer) |
| `D` | Disconnect |
| `Esc` | Disconnect and back |
