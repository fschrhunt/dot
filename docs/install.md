# Install

dot is a single binary. Only git is needed at runtime.

## The installer

```sh
curl -fsSL https://fschrhunt.com/dot/install.sh | sh
```

It downloads the release archive for your system (macOS or Linux, Intel or ARM), checks it
against the release's `checksums.txt`, and installs `dot` into `~/.local/bin`, saying so if
that isn't on your `PATH`. Run `dot update` to update, or the installer again.
It never edits your shell files. Options, after `sh -s --`:

```sh
curl -fsSL https://fschrhunt.com/dot/install.sh | sh -s -- --version v1.0.0 --dir ~/bin
```

`DOT_INSTALL_DIR` and `DOT_VERSION` do the same as `--dir` and `--version`. The script is
[`install.sh`](../install.sh) in dot's repository; read it before you run it if you like.

## Homebrew

dot's repository is its own tap:

```sh
brew tap fschrhunt/dot https://github.com/fschrhunt/dot
brew install fschrhunt/dot/dot
```

The install command is fully qualified: bare `brew install dot` finds an unrelated cask of the
same name. If Homebrew refuses the tap as untrusted, run `brew trust fschrhunt/dot` and install
again. Each release updates the formula, `dot.rb` at the repository root, so
`brew upgrade fschrhunt/dot/dot` brings the latest. The formula is generated, not hand-edited:
`scripts/formula.sh` writes it from the release's checksums.

## With Go

Go 1.26 or newer can build and install dot:

```sh
go install github.com/fschrhunt/dot/cmd/dot@latest
```

Put `$(go env GOPATH)/bin` on your `PATH`. A binary built with go install reports `dev`;
release binaries report their tag.

## By hand

Download `dot_VERSION_OS_ARCH.tar.gz` for your system and `checksums.txt` from
[Releases](https://github.com/fschrhunt/dot/releases/latest), check the archive's SHA-256
(`sha256sum` on Linux, `shasum -a 256` on macOS), extract it and put `dot` on your `PATH`. Each
archive's build provenance can be verified with
`gh attestation verify dot_VERSION_OS_ARCH.tar.gz -R fschrhunt/dot`.

## From source

```sh
git clone https://github.com/fschrhunt/dot
cd dot
go build -o dot ./cmd/dot
mkdir -p ~/.local/bin
cp dot ~/.local/bin/dot
```

The starter setup is embedded. You can move the binary without keeping the checkout.

After replacing a previous installation, run `dot timer` again. The timer keeps the binary's
absolute path. Keep the binary there while the timer is installed.

## Start

```sh
dot init
```

See [getting started](getting-started.md).
