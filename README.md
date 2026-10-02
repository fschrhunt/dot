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
Codex, OpenCode and others call theirs `AGENTS.md`. To dot they are one file with several names,
and so is each skill: an edit under any name reaches the rest, on every machine.

## Install

```sh
curl -fsSL https://fschrhunt.com/dot/install.sh | sh            # macOS and Linux
brew tap fschrhunt/dot https://github.com/fschrhunt/dot && brew install fschrhunt/dot/dot
go install github.com/fschrhunt/dot/cmd/dot@latest             # Go 1.26 or newer
```

One binary, and only git at runtime. See [Install](docs/install.md) for options,
checking a download by hand, and building from source.

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
