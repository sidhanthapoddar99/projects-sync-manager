# Projects Sync Manager (PSM)

A fast, single-binary TUI tool to manage and sync dozens of Git repositories across multiple machines.

Scan a directory tree, see the sync status of every repo at a glance, pull/push with one keystroke, filter by status, and use reference files to replicate your project structure on a new machine.

> For a comprehensive usage guide, see **[GUIDE.md](GUIDE.md)**.

---

## Quick Start

One command — auto-detects your OS/arch, downloads the latest binary, caches it, and runs it. Subsequent runs skip the download.

**Linux / macOS:**
```bash
curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/install.sh | sh
```

For a custom directory:

```bash
curl -sL https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/install.sh | sh -s -- -d ~/projects
```

**Windows (PowerShell):**
```powershell
irm https://raw.githubusercontent.com/sidhanthapoddar99/projects-sync-manager/master/install.ps1 | iex
```

That's it. The binary is cached in your temp folder (`/tmp/psm-cache/` or `%TEMP%\psm-cache\`) and reused on future runs. A new version is downloaded automatically when a release is published.

<details>
<summary>Other installation methods</summary>

```bash
# --- Direct download (no caching, fresh every time) ---

# Linux (amd64)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# macOS (Apple Silicon)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-darwin-arm64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# --- Install to PATH for permanent use ---
sudo curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-amd64 -o /usr/local/bin/psm && sudo chmod +x /usr/local/bin/psm

# --- Install via Go ---
go install github.com/sidhanthapoddar99/projects-sync-manager/cmd@latest
```

</details>

---

## Usage

```
psm [flags]

Flags:
  -d <path>    Target directory to scan (default: current directory)
  -h <depth>   Max depth for git repo discovery (default: 3)
  --version    Print version and exit
```

---

## Features

### Tree View with Status Indicators

```
my-project/     ● ✓                    # synced (green)
api-server/     ● △ ↑2                 # 2 commits to push (yellow)
webapp/         ● ✗ ~4 …2              # uncommitted changes (red)
experiments/    ● ?                     # no remote (blue)
data-scripts/   ● ✓ ≠                  # name mismatch (yellow ≠)
some-folder/    ○                       # not a git repo (grey)
```

| Symbol | Meaning |
|--------|---------|
| `●` / `○` | Git repo / not a git repo |
| `✓` `△` `✗` `?` | Synced / partial / dirty / no remote |
| `≠` | Folder name differs from remote repo name |
| `↑N` `↓N` | Commits to push / pull |
| `+N` `~N` `…N` | Staged / unstaged / untracked files |

### Command Palette (`/`)

A VS Code-style fuzzy search bar for all available commands. Type to search, `Tab` to auto-complete, `Enter` to execute.

### Filters (`F`)

Multi-select filter panel to focus on repos that need attention. Combine filters with OR logic — enable "Dirty" and "Partial" to see all repos that need work.

Available filters: Dirty, Partial, Synced, No Remote, Ahead, Behind, Name Mismatch.

### Repo Detail Panel (`Enter`)

Interactive panel with per-branch sync control. Switch between branches and actions with `Tab`. Sync individual branches without switching — even non-current branches.

### Reference Files (`f`)

Generate a portable snapshot of your project structure. Load it on another machine to see what's missing, clone repos individually or in bulk, and sync matched repos.

### Conservative Sync (`s`)

Pull or push with safety checks — blocks on uncommitted changes and diverged branches. Uses `--ff-only` for pulls. No auto-conflict resolution.

---

## Keyboard Controls

| Key | Action |
|-----|--------|
| `↑/↓` or `k/j` | Navigate siblings |
| `→/←` or `l/h` | Enter/exit directories |
| `Enter` | Open repo detail panel |
| `/` | Command palette |
| `F` | Filter panel |
| `s` | Sync selected repo |
| `r` | Refresh selected repo |
| `R` | Refresh all repos |
| `f` | Reference file menu |
| `c` | Open in VS Code |
| `e` | Open in file explorer |
| `b` | Open in browser |
| `?` | Help |
| `q` | Quit |

See **[GUIDE.md](GUIDE.md)** for all keybindings including detail panel, filter panel, command palette, and compare view.

---

## Building from Source

```bash
# Prerequisites: Go 1.22+, Git

# Build for current platform
go build -ldflags="-s -w" -o psm ./cmd/

# Build for all platforms
mkdir -p dist
for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  GOOS=${pair%/*} GOARCH=${pair#*/} CGO_ENABLED=0 \
    go build -ldflags="-s -w" \
    -o "dist/psm-${pair%/*}-${pair#*/}$([ ${pair%/*} = windows ] && echo .exe)" \
    ./cmd/
done

# Optional: compress linux binaries (~40% smaller)
# Requires upx: sudo apt install upx-ucl
upx --best dist/psm-linux-*
```

---

## Creating a GitHub Release

```bash
# Tag and push — GitHub Actions builds and publishes binaries automatically
git tag -a v0.1.0 -m "Initial release"
git push origin v0.1.0
```

The workflow at `.github/workflows/release.yml` automatically:
1. Builds static binaries for Linux, macOS, and Windows
2. Compresses Linux binaries with UPX
3. Packages source archives (.tar.gz and .zip)
4. Generates SHA256 checksums for all artifacts
5. Creates a GitHub release with everything attached

---

## Ignore File

PSM auto-generates a `.psmignore` file in the scanned directory with sensible defaults (node_modules, build, dist, vendor, __pycache__, etc.). Add custom regex patterns to skip additional directories during scanning.

---

## License

MIT
