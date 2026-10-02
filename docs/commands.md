# Commands

`DOT_HOME` selects the setup folder. `DOT_MACHINE` overrides the short hostname.
Commands validate every known machine. Errors use `dot:` and normally exit 2.
A refused edit or dirty sync exits 1.

## dot / status [path]

```sh
dot status
# last sync: 2026-10-01 12:00:00 laptop up to date; 0 changed
# < take        ~/.codex/AGENTS.md
# - removed     ~/old.conf
# ~ changed     ~/.claude/CLAUDE.md
# + new         ~/.config/app.conf
# ! edited here ~/.zshrc
# ? extra       ~/.config/app/local.conf
```

Status prints what `dot sync` would do. `<` is an edit sync would take into the setup; `!` is a
file sync will not touch until you choose. A held-back take carries its reason on the line,
such as two names of one file edited differently. `dot apply -n` prints what apply alone would do.
The last sync line appears only after a sync. `?` means an extra file under a mapping
with `mirror = false`. A directory creation ends in `/`. `> run` names a mapping's
[`run`](config.md#run) command that apply would run after these changes.
Exit 1 means the plan has work to do. Extras alone exit 0. With no actions, prints `up to date`.

```sh
dot status ~/.gitconfig
# --- live ~/.gitconfig
# +++ dot ~/.gitconfig
# @@ -1 +1 @@
# -old
# +new
```

A path limits diffs to that file or folder. An edit sync would take is shown from the setup's
side, as `--- dot` and `+++ live`. Binary changes print `binary files ... differ`.
A path outside managed destinations is an error.

## add [--only] <path>…

```sh
dot add ~/.gitconfig ~/.claude/CLAUDE.md
# added ~/.gitconfig as home/.gitconfig
# added ~/.claude/CLAUDE.md as agents/instructions.md (shared with every agent that has a place for it)
# + new         ~/.codex/AGENTS.md
```

Copies each path into the setup and applies it. A path under your home folder goes to `home/`.
An agent's instructions or skill goes to `agents/` and is written to the other installed agents;
a path inside a skill adds the whole skill. `--only` stores an agent's path under `home/`, so it
stays with that agent. A folder is added file by file. Needs a version 2 setup.

A file that looks like it holds a credential is left out and named. A `.git` folder is left out
too, and so is a file whose name ends in `.tmpl`, which would become a template: map it under
`[files]`. An agent's whole folder, such as `~/.claude`, is refused: it holds sessions and credentials, so
add the files and skills you want from it. Your whole home folder, and anything inside the
setup, are refused too.

A path outside your home folder is an error; map it under `[files]`. A path the setup already
holds is left as it is, and status shows how the two differ.

## forget <path>…

```sh
dot forget ~/.gitconfig
# forgot ~/.gitconfig; it stays where it is
```

Removes the path from the setup and from dot's record of what it wrote, so the live file stays.
A shared path is forgotten under all of its names. A path mapped in `dot.toml` is not touched;
remove its line there. One file inside a shared skill is refused, since the skill would take it
back: forget the skill, or remove the file from the setup.

## log [path]

```sh
dot log ~/.codex/AGENTS.md
# 5f8f763 2026-10-02 02:04 laptop: ~/.codex/AGENTS.md
# dadc031 2026-10-01 18:20 desktop: ~/.claude/CLAUDE.md
```

Prints the setup's last twenty changes, newest first: the commit, when it was made, and the
machine and paths it names. With a path, only the changes to that file or folder, under any of
its names. The setup is a git repository, so `git -C ~/.dot log` shows the rest.

## undo <path>

```sh
dot undo ~/.codex/AGENTS.md
# undid 5f8f763 laptop: ~/.codex/AGENTS.md for ~/.codex/AGENTS.md; dot sync sends it to the other machines
# ~ changed     ~/.claude/CLAUDE.md
# ~ changed     ~/.codex/AGENTS.md
```

Takes a file or folder back to how the setup had it before its last change, commits that as
`<machine>: undo <path>`, and applies it here. The next sync sends it to the other machines.
Nothing is erased: the undo is one more change, so `dot undo` again brings the path forward.

Undo needs a setup that syncs both ways, since its commit has to reach the remote. A path with
changes that are not synced yet is refused, exit 1; `dot sync` first, or
`dot apply --force` to drop them. A path whose last change is the one that added it is refused
too: `dot forget` is how a path stops being managed.

## agents

```sh
dot agents
# claude     ~/.claude  (installed)
#   instructions   ~/.claude/CLAUDE.md
#   skills         ~/.claude/skills
```

Prints each agent dot knows, whether it is installed here, and where it keeps each kind of
thing. See [agents](agents.md).

## apply [-n] [--force]

```sh
dot apply -n
# ~ changed     ~/.gitconfig
dot apply
# ~ changed     ~/.gitconfig
```

`-n` prints the plan without writing and exits 0.
After writing, apply runs the [`run`](config.md#run) command of each mapping it changed.
`--force` allows replacing files edited at the destination. Review the diff first.
In a setup that takes, apply leaves a new file in a shared folder alone and lists it as
`? extra`; sync takes it.

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

Sync is quiet when it succeeds. It takes live edits, commits, pulls, pushes, then applies; a
version 1 setup only pulls and applies. On failure it prints the same notes to the terminal,
one `dot:` line each, and always records them, success or failure, in `.state/last` and
`.state/sync.log`. See [sync](sync.md).
`--settled`, which the timer uses, leaves a file modified in the last minute for the next run,
and waits while the setup itself was edited that recently.

## take <path>

```sh
dot take ~/.config/zsh
# took ~/.config/zsh/aliases.zsh -> ~/.dot/zsh/aliases.zsh
```

Copies changed files and new files back to the managed source, whatever sync would have held
back. A folder of separately managed files, such as one under `home/`, is taken file by file;
a version 1 setup refuses it and asks for one destination at a time. Does not delete missing source files or commit; the next sync commits. Excludes still
apply. An edit to a rendered file goes into its template when it touches only lines the template
leaves as they are; otherwise take refuses and shows the diff.
The live path must exist and belong to a mapping.

## init [remote]

```sh
dot init
# Created ~/.dot. Next: dot add the files you want managed, then dot sync and dot timer.
```

Creates a version 2 setup with a commented `dot.toml` and initializes git. Refuses an existing
setup folder.

```sh
dot init git@github.com:you/dotfiles.git
```

Clones that remote instead. Ensures `.state/` is ignored locally.

## timer [--remove]

```sh
dot timer
# Installed: dot sync runs every 15 minutes on laptop.
dot timer --remove
# Removed the dot timer.
```

Linux uses a systemd user timer. macOS uses a LaunchAgent.
The interval is 15 minutes unless [`[sync] every`](config.md#syncevery) sets another.
The timer runs the binary at its current absolute path and keeps PATH, DOT_HOME and DOT_MACHINE.
Run it again after moving the binary or changing the interval. An option it does not know is
an error, as it is for `dot sync`, so a mistyped `--remove` installs nothing. `dot install` is the earlier
name for this command and still works. See [sync](sync.md) for inspecting the timer.

## help / -h / --help

```sh
dot help
# dot: the dotfiles manager.
# ... usage ...
# Setup ~/.dot on machine laptop.
# Values ({{name}} in templates, sources and destinations):
#   email = me@example.com
#     set per machine (work: me@work.example; base: me@example.com): differs on purpose; do not unify
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
