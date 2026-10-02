# Getting started

## Create your setup

```sh
dot init
# Created ~/.dot. Next: dot add the files you want managed, then dot sync and dot timer.
```

The setup is a git repository at `~/.dot`. Its folders say where files go:

```text
~/.dot/
  home/       mirrors your home folder: home/.gitconfig is written to ~/.gitconfig
  agents/     shared by every coding agent you have installed
  dot.toml    optional settings
```

## Add your files

```sh
dot add ~/.gitconfig ~/.config/zsh
# added ~/.gitconfig as home/.gitconfig
# added ~/.config/zsh as home/.config/zsh
```

Nothing moves. dot copies each file into the setup and remembers it. From then on, edit the
file where it lives.

## Share one file across your agents

```sh
dot add ~/.claude/CLAUDE.md
# added ~/.claude/CLAUDE.md as agents/instructions.md (shared with every agent that has a place for it)
# + new         ~/.codex/AGENTS.md
# + new         ~/.config/opencode/AGENTS.md
```

Claude Code calls its instructions `CLAUDE.md`; Codex and opencode call theirs `AGENTS.md`. To
dot they are one file with several names. A skill works the same way:

```sh
dot add ~/.claude/skills/review
# added ~/.claude/skills/review as agents/skills/review (shared with every agent that has a place for it)
```

`dot agents` shows which agents dot found and where each keeps things. To keep a file to one
agent, use `dot add --only`. See [agents](agents.md).

## Sync

```sh
dot status
# < take        ~/.codex/AGENTS.md
# ~ changed     ~/.claude/CLAUDE.md
dot sync
```

You, or an agent, edited `~/.codex/AGENTS.md`. `dot sync` takes the edit into the setup, commits
it, pushes it, and writes it under the file's other names. `dot status` shows the plan first.

Run sync on a timer so you never have to:

```sh
dot timer
# Installed: dot sync runs every 15 minutes on laptop.
```

## A second machine

Give the setup a private remote, then clone it on the other machine:

```sh
ssh server git init --bare dot.git
git -C ~/.dot remote add origin server:dot.git
git -C ~/.dot push -u origin HEAD
```

```sh
dot init server:dot.git
dot sync
dot timer
```

An edit made on either machine reaches the other on its next sync. If a file there already
differs from the setup, dot leaves it alone and `dot status` marks it `! edited here`:
`dot take <path>` keeps that machine's version, and `dot apply --force` replaces it.

## What differs between machines

- A file whose name ends in `.tmpl` is a template. `home/.gitconfig.tmpl` is written to
  `~/.gitconfig` with each `{{name}}` filled in from `[values]` and `[machine.<name>]`.
- `[only]` limits a path to some machines or some agents.
- An agent that is not installed on a machine is skipped there.

See [configuration](config.md) and [sync](sync.md). Do not put credentials in a setup you share.
