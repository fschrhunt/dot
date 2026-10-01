# Commands

`DOT_HOME` selects the setup folder. `DOT_MACHINE` overrides the short hostname.
Commands validate every known machine. Errors use `dot:` and normally exit 2.
A refused edit or dirty sync exits 1.

## dot / status [path]

```sh
dot status
# last sync: 2026-10-01 12:00:00 laptop up to date; 0 changed
# - removed     ~/old.conf
# ~ changed     ~/.gitconfig
# + new         ~/.config/app.conf
# ! edited here ~/.zshrc
# ? extra       ~/.config/app/local.conf
```

The last sync line appears only after a sync. `?` means an extra file under a mapping
with `mirror = false`. A directory creation ends in `/`.
Exit 1 means the plan has work to do. Extras alone exit 0. With no actions, prints `up to date`.

```sh
dot status ~/.gitconfig
# --- live ~/.gitconfig
# +++ dot ~/.gitconfig
# @@ -1 +1 @@
# -old
# +new
```

A path limits diffs to that file or folder. Binary changes print `binary files ... differ`.
A path outside managed destinations is an error.

## apply [-n] [--force]

```sh
dot apply -n
# ~ changed     ~/.gitconfig
dot apply
# ~ changed     ~/.gitconfig
```

`-n` prints the plan without writing and exits 0.
`--force` allows replacing files edited at the destination. Review the diff first.

```sh
dot apply
# dot: edited here: ~/.zshrc (dot take ~/.zshrc, or dot apply --force)
dot take ~/.zshrc
# took ~/.zshrc -> ~/.dot/zshrc
dot apply --force
```

The last command uses the source version. If take already copied the edit back, no write is needed.
A refused path also protects actions below it and actions replacing a folder above it.

## sync

```sh
dot sync
dot status
# last sync: 2026-10-01 12:15:00 laptop pulled; 2 changed
# up to date
```

Sync is quiet. It pulls with `--ff-only`, optionally pushes, then applies.
Read `.state/last` or `.state/sync.log` for results. See [sync](sync.md).

## take <path>

```sh
dot take ~/.config/zsh
# took ~/.config/zsh/aliases.zsh -> ~/.dot/zsh/aliases.zsh
```

Copies changed files and new files back to the managed source. Does not delete missing source
files or commit. Excludes still apply. Template destinations are refused with a diff.
The live path must exist and belong to a mapping.

## init [remote]

```sh
dot init
# Created ~/.dot. Next: edit ~/.dot/dot.toml, run dot apply, then dot install.
```

Copies the embedded example and initializes git. Refuses an existing setup folder.

```sh
dot init server:dot.git
```

Clones that remote instead. Ensures `.state/` is ignored locally.

## install [--remove]

```sh
dot install
# Installed: dot sync runs every 15 minutes on laptop.
dot install --remove
# Removed the dot timer.
```

Linux uses a systemd user timer. macOS uses a LaunchAgent.
The timer runs the binary at its current absolute path and keeps PATH, DOT_HOME and DOT_MACHINE.
Run install again after moving the binary. See [sync](sync.md) for inspecting the timer.

## help / -h / --help

```sh
dot help
# dot: your setup, the same on every machine.
# ... usage ...
# Setup ~/.dot on machine laptop.
# Values ({{name}} in templates, sources and destinations):
#   workspace_root = ~/Code
#     set per machine (server: ~/src; base: ~/Code): differs on purpose; do not unify
# Sources (edit these in the setup, not the destinations):
#   instructions.md -> ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md  (template)
```

Works before init. Includes raw source and destination placeholders for inspecting the setup.

## version / --version

```sh
dot version
# dev
dot --version
# dev
```

Release binaries print their version tag. Source builds print `dev` unless a version is supplied
with `-ldflags '-X main.version=v1.0.0'`. This command works without a setup.
