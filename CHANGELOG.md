# dot releases

## v1.0.0 · 2026-10-02

- Install with `curl -fsSL https://fschrhunt.com/dot/install.sh | sh` (checksum-verified, into
  `~/.local/bin`) or Homebrew, from dot's own repository: `brew tap fschrhunt/dot
  https://github.com/fschrhunt/dot && brew install dot`.
- Releases carry build provenance (`gh attestation verify`), run CI's checks and `govulncheck`
  first, and are installed for real on macOS and Linux. `scripts/release.sh` releases in two runs.
- `dot update [--check]` replaces an installer or archive install with the latest release, after
  checking its checksum; `--check` only reports. Homebrew and go install are pointed to their own
  update. In a terminal, dot mentions a newer release at most once a day (`DOT_NO_UPDATE_CHECK=1`
  stops it).
- Fixed: `dot sync` no longer commits or pushes `.state/`. A setup that does not ignore it gets
  the rule added to `.git/info/exclude`, and one where `.state/` is already tracked is refused
  with the way out named, instead of fighting every other machine's pull.
- Fixed: an `[only]` rule that names no agents or machines, a `[files]` mapping with an empty
  `machines` list, and a rule on a path that is not in the setup are errors, instead of
  silently emptying the path's destinations.
- Fixed: `version = 0` and other versions below 1 are errors, instead of silently managing
  nothing.
- Fixed: a `written.json` dot cannot read names the file and what to do with it, instead of a
  raw parser error.
- A failed `dot sync` prints its notes to the terminal, one `dot:` line each, as well as
  recording them in `.state/last` and `.state/sync.log`. It used to exit non-zero in silence.
- `dot status` shows why a take is held back on the plan line itself, such as two names of one
  file edited differently.
- Deleting a file under `home/` prunes the empty folders above it, as mapped folders already did.
- dot.toml accepts TOML 1.1: inline tables may have a trailing comma and span lines. Built with
  Go 1.27; building from source needs Go 1.26 or newer.
- Version 2 setups (`version = 2` in `dot.toml`, and every setup `dot init` creates):
  - Two-way sync. `dot sync` takes an edit made to a live file into the setup, commits it as
    `<machine>: <paths>`, pulls with rebase, pushes, and applies. It never merges and never takes
    a deletion; a file changed on both sides, an edit that adds something shaped like a
    credential, and an edit to a line a template fills in are left alone and reported.
  - One file, several names. An edit under one name is written to the others, and an edit to a
    rendered file goes back into its template.
  - The setup's folders are its configuration: `home/` mirrors the home folder and `agents/` is
    shared with every installed agent, so `agents/instructions.md` is written as `CLAUDE.md` for
    Claude Code and `AGENTS.md` for Codex, OpenCode and Pi. `dot.toml` is optional.
  - `[agent.<name>]` adds an agent or changes a built-in one, and `[only]` limits a path to some
    agents or machines. A name ending in `.tmpl` is a template.
  - `dot add` and `dot forget` start and stop managing a path. `dot agents` prints the agents,
    which are installed, and where each keeps things.
  - `dot status` prints what `dot sync` would do, with `<` for an edit it would take.
  - `[sync] take = false` keeps a machine one-way. `push` defaults to true.
  - A conflicting rebase is undone and nothing is pushed; `dot status` leads with it until a pull
    succeeds. Sync refuses to run while you are resolving it and leaves your rebase alone.
  - Sync does not commit a line that looks like a credential, however it reached the setup, or a
    setup that no longer loads. The timer waits while a file in the setup was edited in the last
    minute.
  - `dot apply` leaves a new file in a shared folder for sync to take, and lists it as `? extra`.
  - `dot add` leaves out a file that looks like it holds a credential and refuses an agent's
    whole folder. `dot forget` refuses one file inside a shared skill.
  - `.git` is always excluded, so a skill that is a git clone is shared without its repository
    instead of being committed as a submodule that reaches no other machine.
  - `dot status <path>` shows what a pending take would change in the setup.
  - Excluding a shared file such as `agents/instructions.md` leaves the copies already written
    under each agent's own name.
  - An `[only]` rule on a skill keeps the machines a rule on its folder set.
  - A kind's path in `[agent.<name>]` must stay inside the agent's folder.
  - `dot add` refuses a name ending in `.tmpl`, which would become a template for another path.
  - `dot take` of a folder under `home/` takes every managed file in it.
  - Two agents whose folders are the same real path, one linked to the other, get one copy.
  - Excluding a name under `home/` or `agents/` that dot already wrote leaves the live file.
- `dot log [path]` prints the setup's last changes, or one path's. `dot undo <path>` takes a path
  back to before its last change, as a new commit, and applies it.
- An edit to a rendered file is taken when a value in it holds a newline, and when the template
  has no final newline.
- `dot timer` and `dot sync` reject an option they do not know, so a mistyped `--remove` no
  longer installs the timer.
- Fixed: an exclude pattern with a non-ASCII character, such as `café`, now matches.
- Fixed: a source written as `a` and as `./a` is one source.
- Fixed: a machine named only by `[sync.machine.<name>]` has its sources validated.
- `dot take` carries an edit to a rendered file into its template when it touches only lines the
  template leaves as they are. It used to refuse every template destination.
- Version 1 setups behave as before.
- The timer runs `dot sync --settled`, which leaves a file modified in the last minute for its
  next run.
- Sync refuses to replace a file that changed while it was running.
- Faster with exclude patterns: each is compiled once. Status on 2,000 files with six patterns
  went from 183 ms to 54 ms.
- Faster with many mappings: destinations are checked against each other in one pass.
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
