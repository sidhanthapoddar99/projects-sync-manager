# TODO

## Reference File / Load Feature

### ~~Detect repos present under a different directory~~ ✅ Done
Relocated repos are now detected via URL matching and shown with `[⇄]` indicator in purple. Right panel shows both expected and actual paths.

### Handle multiple remotes
Currently PSM only considers `origin` for remote URL matching and sync operations. Repos can have multiple remotes (e.g. `origin`, `upstream`, `fork`).

**Proposed behavior:**
- Store all remotes in the reference file, not just origin
- During comparison, match against ANY remote URL (not just origin)
- In the detail view, show all remotes with labels
- Let the user pick which remote to sync with (default: origin)
- Reference file format could expand to:
  ```json
  {
    "relative_path": "my-project",
    "remotes": {
      "origin": "https://github.com/user/my-project",
      "upstream": "https://github.com/org/my-project"
    }
  }
  ```

## Peer Sync

- [ ] Peer-initiated refresh request — allow peer to request you to refresh a specific repo
- [ ] Bulk remote sync — sync all matched repos on the peer in one action
- [ ] Connection resilience — auto-reconnect on temporary network drops
- [ ] Multiple peers — support more than one simultaneous connection
- [ ] Transfer reference files over the WebSocket instead of manual copying

## Other Ideas

- [ ] Submodule handling — treat as separate repos or ignore?
- [ ] Stash-pull-unstash flow for repos with uncommitted changes
- [ ] Reference file location option — store in `~/.config/psm/` instead of scanned dir
- [ ] Bulk branch sync — sync all branches across all repos (with safety checks)
- ~~[ ] Filtering/search in the tree view (e.g. only show dirty repos)~~ ✅ Done (filter panel with 7 filter types)
- [ ] Show version number and build date in the app (e.g. in help screen or status bar) — inject via `-ldflags` at build time
