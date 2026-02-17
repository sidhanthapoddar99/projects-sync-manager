# Projects Sync Manager (PSM)

A fast, single-binary TUI tool to manage and sync dozens of Git repositories across multiple machines.

Scan a directory tree, see the sync status of every repo at a glance, pull/push with one keystroke, and use reference files to replicate your project structure on a new machine.

---

## Quick Start

### Use Once and Discard (download to /tmp, runs, gone on reboot)

**Linux (amd64)**:
```bash
curl -sL https://github.com/<user>/psm/releases/latest/download/psm-linux-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects
```

**Linux (arm64)**:
```bash
curl -sL https://github.com/<user>/psm/releases/latest/download/psm-linux-arm64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects
```

**macOS (Apple Silicon)**:
```bash
curl -sL https://github.com/<user>/psm/releases/latest/download/psm-darwin-arm64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects
```

**macOS (Intel)**:
```bash
curl -sL https://github.com/<user>/psm/releases/latest/download/psm-darwin-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm -d ~/projects
```

**Windows (PowerShell)**:
```powershell
Invoke-WebRequest -Uri https://github.com/<user>/psm/releases/latest/download/psm-windows-amd64.exe -OutFile $env:TEMP\psm.exe; & $env:TEMP\psm.exe -d C:\projects
```

### Use a Specific Version

Replace `latest` with the version tag:
```bash
curl -sL https://github.com/<user>/psm/releases/download/v0.2.0/psm-linux-amd64 -o /tmp/psm && chmod +x /tmp/psm && /tmp/psm
```

### Download and Keep (reusable)

```bash
sudo curl -sL https://github.com/<user>/psm/releases/latest/download/psm-linux-amd64 -o /usr/local/bin/psm && sudo chmod +x /usr/local/bin/psm
```

Now just run `psm` from anywhere.

### Install via Go

```bash
go install github.com/<user>/psm@latest
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
| `Up/Down` | Navigate between folders |
| `Right` | Expand / enter directory |
| `Left` | Collapse / go to parent |
| `S` | Sync selected repo (pull or push) |
| `C` | Open in VS Code |
| `E` | Open in file explorer |
| `B` | Open remote repo in browser |
| `R` | Refresh all statuses |
| `F` | Reference file menu |
| `Q` | Quit |
| `?` | Help |

### Status Indicators

```
my-project/   ● ✓ ↑0 ↓0        # fully synced
api-server/   ● △ ↑2 ↓0        # 2 commits to push
webapp/       ● ✗ +1 ~4 …2     # uncommitted changes
experiments/  ● ?               # no remote
some-folder/  ○                 # not a git repo
```

---

## Building from Source

### Prerequisites

- Go 1.21+
- Git
- (Optional) [UPX](https://upx.github.io/) for binary compression

### Build for Current Platform

```bash
go build -ldflags="-s -w" -o psm ./cmd/
```

### Build for All Platforms

```bash
# Linux amd64
GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o dist/psm-linux-amd64 ./cmd/

# Linux arm64
GOOS=linux GOARCH=arm64 go build -ldflags="-s -w" -o dist/psm-linux-arm64 ./cmd/

# macOS Apple Silicon
GOOS=darwin GOARCH=arm64 go build -ldflags="-s -w" -o dist/psm-darwin-arm64 ./cmd/

# macOS Intel
GOOS=darwin GOARCH=amd64 go build -ldflags="-s -w" -o dist/psm-darwin-amd64 ./cmd/

# Windows
GOOS=windows GOARCH=amd64 go build -ldflags="-s -w" -o dist/psm-windows-amd64.exe ./cmd/
```

### Compress with UPX (optional, ~40% smaller)

```bash
upx --best dist/psm-*
```

---

## Creating a GitHub Release

### 1. Tag the Commit

```bash
git tag -a v0.1.0 -m "Initial release"
git push origin v0.1.0
```

### 2. Build All Binaries

```bash
mkdir -p dist
for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
  GOOS=${pair%/*} GOARCH=${pair#*/} go build -ldflags="-s -w" -o "dist/psm-${pair%/*}-${pair#*/}$([ ${pair%/*} = windows ] && echo .exe)" ./cmd/
done
upx --best dist/psm-*    # optional
```

### 3. Create the Release

```bash
gh release create v0.1.0 dist/* --title "v0.1.0" --notes "Initial release"
```

Or via GitHub UI: **Releases** > **Draft a new release** > select tag > upload binaries.

### Automating with GitHub Actions

Add `.github/workflows/release.yml`:

```yaml
name: Release
on:
  push:
    tags: ["v*"]

permissions:
  contents: write

jobs:
  release:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version: "1.21"

      - name: Build binaries
        run: |
          mkdir -p dist
          for pair in linux/amd64 linux/arm64 darwin/amd64 darwin/arm64 windows/amd64; do
            GOOS=${pair%/*} GOARCH=${pair#*/} go build -ldflags="-s -w" \
              -o "dist/psm-${pair%/*}-${pair#*/}$([ ${pair%/*} = windows ] && echo .exe)" ./cmd/
          done

      - name: Compress binaries
        run: |
          sudo apt-get install -y upx
          upx --best dist/psm-* || true

      - name: Create release
        uses: softprops/action-gh-release@v2
        with:
          files: dist/*
```

Now every `git tag -a vX.Y.Z && git push origin vX.Y.Z` automatically builds and publishes release binaries.

---

## Reference Files

Generate a snapshot of your project structure:
```
Press F > Generate
```

This creates `projects-ref.json` - a portable file listing all git repos and their remote URLs. Copy it to another machine, load it with `F > Load`, and PSM shows what's missing, what's extra, and lets you clone missing repos in one keystroke.

---

## License

MIT
