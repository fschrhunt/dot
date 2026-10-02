# dot releases

## Unreleased

- `dot timer` is the new name for `dot install`, which still works.
- The macOS agent is now labeled `com.fschrhunt.dot`. `dot timer` removes an agent installed
  under the earlier label `dot`.
- `[sync] every` sets how often the timer runs `dot sync`, such as `"1m"` or `"1h"`. It defaults
  to 15 minutes, as before. Run `dot timer` again after changing it.
- `[sync] after_boot` sets how long after boot the Linux timer first runs. It defaults to 2 minutes.
- `[sync] timeout` and `connect_timeout` replace the fixed 60-second git deadline and 5-second
  ssh connection timeout. The defaults are unchanged.
- `[sync.machine.<name>]` overrides any `[sync]` setting on one machine.
- A mapping's `run` command runs after apply or sync changes anything in that mapping. `dot status`
  lists it as `> run`.
- Fixed: `dot sync` no longer replaces your ssh command. It adds its options to `GIT_SSH_COMMAND`
  or `core.sshCommand` when you have set one.
- Fixed: a git command that reaches the timeout is stopped even when its ssh process lingers.
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
- Fixed: `dot timer` keeps `DOT_MACHINE` in the timer, and escapes quotes, backslashes, `%` and
  `$` in the systemd unit.
- Fixed: wrong value types in `dot.toml` (such as `mirror = "false"` or `version = "1"`) are a
  `dot:` error naming the field.
- Fixed: `dot status <path>` says when binary files differ instead of showing an empty diff.
- Fixed: `dot take` of a managed folder that no longer exists is an error instead of doing nothing.
- The install steps create `~/.local/bin` and say it must be on `PATH`.
