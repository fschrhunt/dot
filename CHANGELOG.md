# dot releases

## Unreleased

- Brand assets: the dot wordmark and logo in black and white under `assets/`, and the wordmark in
  the README header.
- A failed pull or push makes `dot sync` exit 1. It still applies the local setup.
- Rewritten in Go as a single binary with an embedded starter setup. No Python needed.
- Added worked examples in docs/ for installation, configuration, commands and syncing.
- Added `dot version` and `--version`, plus release archives for Linux and macOS on amd64 and arm64.

- First release: `status`, `apply`, `sync`, `take`, `init`, `install` and `help`, driven by one
  plan from `dot.toml`; templates with per-machine values; mirrored folders; files edited in place
  are never overwritten or deleted without `--force`.
- Fixed: a managed folder replaced by a symlink is reported as edited, and nothing beneath it is
  written or deleted through the link; actions beneath any refused path are skipped too.
- Fixed: adding an exclude no longer deletes a file dot wrote earlier; excluded files are never deleted.
- Fixed: a new folder takes its source folder's permissions instead of the umask default.
- Fixed: validation resolves symlinked parent folders, so two mappings cannot write one file
  through an alias, and catches destinations nested under `/`.
- Fixed: a file that becomes a folder (or a folder that becomes a file) in the source converges in
  one apply. The plan now lists deletions before writes.
- Fixed: pruning empty folders keeps any folder the source still has.
- Fixed: `dot install` keeps `DOT_MACHINE` in the timer, and escapes quotes, backslashes, `%` and
  `$` in the systemd unit.
- Fixed: wrong value types in `dot.toml` (such as `mirror = "false"` or `version = "1"`) are a
  `dot:` error naming the field.
- Fixed: `dot status <path>` says when binary files differ instead of showing an empty diff.
- Fixed: `dot take` of a managed folder that no longer exists is an error instead of doing nothing.
- The install steps create `~/.local/bin` and say it must be on `PATH`.
