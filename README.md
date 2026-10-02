<p align="center">
  <picture>
    <source srcset="assets/white/wordmark.svg" media="(prefers-color-scheme: dark)">
    <source srcset="assets/black/wordmark.svg" media="(prefers-color-scheme: light)">
    <img src="assets/black/wordmark.svg" alt="dot" height="48">
  </picture>
</p>
<p align="center">The dotfiles manager.</p>
<p align="center"><a href="https://github.com/fschrhunt/dot/actions/workflows/ci.yml"><img src="https://github.com/fschrhunt/dot/actions/workflows/ci.yml/badge.svg" alt="CI"></a></p>

---

Edit your dotfiles where they live. dot takes each edit into a private git repository, `~/.dot`,
and carries it to your other machines.

It treats your coding agents as one audience. Claude Code calls its instructions `CLAUDE.md`;
Codex, opencode and others call theirs `AGENTS.md`. To dot they are one file with several names,
and so is each skill: an edit under any name reaches the rest, on every machine.

## Install

Download a Linux or macOS binary from [Releases](https://github.com/fschrhunt/dot/releases),
extract it, and put `dot` on your PATH. Only git is needed at runtime. With Go 1.25 or newer:

```sh
go install github.com/fschrhunt/dot/cmd/dot@latest
```

## Start

```sh
dot init
dot add ~/.gitconfig ~/.config/zsh    # stored under home/, which mirrors your home folder
dot add ~/.claude/CLAUDE.md           # stored under agents/, and written as AGENTS.md for the rest
dot sync                              # take edits, commit, pull, push, apply
dot timer                             # and keep doing it, every 15 minutes
```

`dot status` shows what a sync would do before it does it. dot never merges and never syncs a
deletion: a file changed on two sides is reported and left alone until you choose. Every change
is a commit, so `dot log <path>` shows who changed a file and `dot undo <path>` takes it back.
See [the docs](docs/README.md) for installation,
configuration, commands and syncing a second machine.

MIT licensed.
