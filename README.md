# Projects Sync Manager (PSM)

A fast, single-binary TUI tool to manage and sync dozens of Git repositories across multiple machines.

Scan a directory tree, see the sync status of every repo at a glance, pull/push with one keystroke, and use reference files to replicate your project structure on a new machine.

---

## Quick Start

```bash
# --- Use once and discard (downloaded to /tmp, gone on reboot) ---

# Linux (amd64)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# Linux (arm64)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-arm64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# macOS (Apple Silicon)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-darwin-arm64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# macOS (Intel)
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-darwin-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects

# --- Use a specific version (replace v0.1.0 with desired tag) ---
curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/download/v0.1.0/psm-linux-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm

# --- Download and keep (install to PATH for reuse) ---
sudo curl -sL https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-linux-amd64 -o /usr/local/bin/psm && sudo chmod +x /usr/local/bin/psm

# --- Windows (PowerShell) ---
# Invoke-WebRequest -Uri https://github.com/sidhanthapoddar99/projects-sync-manager/releases/latest/download/psm-windows-amd64.exe -OutFile $env:TEMP\psm.exe; & $env:TEMP\psm.exe -d C:\projects

# --- Install via Go ---
# go install github.com/sidhanthapoddar99/projects-sync-manager/cmd@latest
```

---

## Usage

```
psm [flags]

Flags:
  -d <path>    Target directory to scan (default: current directory)
  -h <depth>   Max depth for git repo discovery (default: 3)
```

### Keyboard Controls

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate siblings (same directory level) |
| `→` | Enter / expand directory |
| `←` | Collapse directory / go to parent |
| `Enter` | Open repo detail panel (interactive) |
| `S` | Sync selected repo (pull or push) |
| `C` | Open in VS Code |
| `E` | Open in file explorer |
| `B` | Open remote repo in browser |
| `R` | Refresh all statuses (fetches from remote) |
| `F` | Reference file menu |
| `Q` | Quit |
| `?` | Help |

### Repo Detail Panel (Enter on a git repo)

| Key | Action |
|-----|--------|
| `↑/↓` | Navigate actions and branches |
| `Enter` | Execute selected action |
| `Esc/←` | Back to tree view |

### Status Indicators

```
my-project/   ● ✓                # green — fully synced
api-server/   ● △ ↑2 ↓0         # yellow — 2 commits to push
webapp/       ● ✗ +1 ~4 …2      # red — uncommitted changes
experiments/  ● ?                # blue — no remote
some-folder/  ○                  # grey — not a git repo
```

| Symbol | Meaning |
|--------|---------|
| `●` / `○` | Git repo / not a git repo |
| `✓` `△` `✗` `?` | Synced / partial / dirty / no remote |
| `↑N` `↓N` | Commits to push / pull |
| `+N` `~N` `…N` | Staged / unstaged / untracked files |

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

# Or create a release manually with gh CLI
# gh release create v0.1.0 dist/* --title "v0.1.0" --notes "Initial release"
```

The workflow at `.github/workflows/release.yml` automatically:
1. Builds static binaries for Linux, macOS, and Windows
2. Compresses Linux binaries with UPX
3. Creates a GitHub release with all binaries attached

---

## Reference Files

Generate a snapshot of your project structure:
```
Press F > G (Generate)
```

This creates `projects-ref.json` — a portable file listing all git repos and their remote URLs. Copy it to another machine, load it with `F > L` (Load), and PSM shows what's missing, what's extra, and lets you clone missing repos in one keystroke.

---

## Ignore File

PSM auto-generates a `.psmignore` file in the scanned directory with sensible defaults (node_modules, build, dist, vendor, __pycache__, etc.). Add custom regex patterns to skip additional directories during scanning.

---

## License

MIT
