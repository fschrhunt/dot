# Install

## Release binary

Download an archive from [Releases](https://github.com/fschrhunt/dot/releases).
Choose `linux` or `darwin` (macOS), and `amd64` or `arm64`.
Apple Silicon Macs use `darwin_arm64`. Intel Macs use `darwin_amd64`.

For example, after downloading a Linux amd64 archive:

```sh
tar -xzf dot_v1.0.0_linux_amd64.tar.gz
mkdir -p ~/.local/bin
mv dot ~/.local/bin/dot
export PATH="$HOME/.local/bin:$PATH"
dot version
```

Use the actual downloaded filename. Add the PATH line to your shell profile.
Install git if `git --version` fails. dot needs nothing else at runtime.

## go install

With Go 1.26 or newer:

```sh
go install github.com/fschrhunt/dot/cmd/dot@latest
export PATH="$(go env GOPATH)/bin:$PATH"
dot help
```

A binary built with go install reports `dev`. Release binaries report their tag.

## Build from source

```sh
git clone https://github.com/fschrhunt/dot
cd dot
go build -o dot ./cmd/dot
mkdir -p ~/.local/bin
cp dot ~/.local/bin/dot
export PATH="$HOME/.local/bin:$PATH"
```

The starter setup is embedded. You can move the binary without keeping the checkout.

After replacing a previous installation, run `dot timer` again. The timer will use
that binary's absolute path. Keep the binary there while the timer is installed.
