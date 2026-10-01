# Getting started

## Create your setup

```sh
dot init
# Created ~/.dot. Next: edit ~/.dot/dot.toml, run dot apply, then dot install.
$EDITOR ~/.dot/dot.toml
```

The example renders `instructions.md` into `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md`.
Change the destinations and values to suit your setup.

## First apply

```sh
dot status
# + new         ~/.claude/CLAUDE.md
# + new         ~/.codex/AGENTS.md
dot apply
dot status
# up to date
```

A live file that already exists with different contents is protected. Review its diff with
`dot status <path>`. Use `dot apply --force` when you want the source to replace it.

## Add a file, then take an edit

Create a source and mapping first. take works only on managed paths.

```sh
cp ~/.gitconfig ~/.dot/gitconfig
cat >> ~/.dot/dot.toml <<'TOML'
[files]
"gitconfig" = "~/.gitconfig"
TOML
dot apply
$EDITOR ~/.gitconfig
dot take ~/.gitconfig
# took ~/.gitconfig -> ~/.dot/gitconfig
cd ~/.dot
git add dot.toml gitconfig
git commit -m 'Manage git config'
```

If `[files]` already exists, add the mapping to that table instead of adding another header.
Do not put credentials in a setup you share.

## Sync a second machine

Create a private remote. This example uses a bare repo on your server:

```sh
ssh server git init --bare dot.git
cd ~/.dot
git add .
git commit -m 'My setup'
git remote add origin server:dot.git
git push -u origin HEAD
dot install
```

On the second machine, install the binary, then:

```sh
dot init server:dot.git
dot apply
dot install
```

Add `[machine.<name>]` overrides before using machine-specific sources. Validation checks every
known machine. See [configuration](config.md) and [sync](sync.md).
