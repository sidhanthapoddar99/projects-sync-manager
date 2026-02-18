# TODO

## Reference File / Load Feature

### Detect repos present under a different directory
When loading a reference file and comparing, if a repo's remote URL matches a local repo but the relative path differs (i.e. the repo was moved/renamed), PSM currently marks it as `[--]` missing AND `[++]` extra instead of recognizing it as the same repo in a different location.

**Proposed behavior:**
- During comparison, match by remote URL first, then by path
- If a remote URL matches but the path differs, show a new indicator like `[~~]` (relocated)
- Right panel should display both paths: expected (from reference) vs actual (local)
- Offer an action to update the reference file with the new path

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

## Other Ideas

- [ ] Submodule handling — treat as separate repos or ignore?
- [ ] Stash-pull-unstash flow for repos with uncommitted changes
- [ ] Reference file location option — store in `~/.config/psm/` instead of scanned dir
- [ ] Bulk branch sync — sync all branches across all repos (with safety checks)
- [ ] Filtering/search in the tree view (e.g. only show dirty repos)
