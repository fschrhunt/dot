<p align="center">
  <picture>
    <source srcset="assets/white/wordmark.svg" media="(prefers-color-scheme: dark)">
    <source srcset="assets/black/wordmark.svg" media="(prefers-color-scheme: light)">
    <img src="assets/black/wordmark.svg" alt="dot" height="48">
  </picture>
</p>
<p align="center">Your setup, the same on every machine.</p>
<p align="center"><a href="https://github.com/fschrhunt/dot/actions/workflows/ci.yml"><img src="https://github.com/fschrhunt/dot/actions/workflows/ci.yml/badge.svg" alt="CI"></a></p>

---

Keep your dotfiles, agent instructions, skills and small tools in a private git folder,
`~/.dot`. dot copies them wherever `dot.toml` says. A timer pulls and applies your commits
on each machine. Templates supply machine-specific values. Edited live files stay protected.

## Install

Download a Linux or macOS binary from [Releases](https://github.com/fschrhunt/dot/releases),
extract it, and put `dot` on your PATH. Only git is needed at runtime. With Go 1.25 or newer:

```sh
go install github.com/fschrhunt/dot/cmd/dot@latest
```

## Start

```sh
dot init
$EDITOR ~/.dot/dot.toml
dot status
dot apply
dot install
```

Commit your setup changes before syncing. See [the docs](docs/README.md) for installation,
configuration, commands and syncing a second machine.

MIT licensed.
