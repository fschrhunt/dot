# Contributing

Smallness is the point. dot is a Go binary with one TOML dependency and git at runtime.
An extra package, dependency or option has to earn its place. Removing complexity is welcome.

## Setup and tests

Use Go 1.25 or newer:

```sh
git clone https://github.com/fschrhunt/dot
cd dot
go build ./...
go vet ./...
go test ./...
gofmt -l .
```

Fetch the module once with `go mod download`. Tests then run offline in temp homes and local
bare git repos. Add one focused test per behavior you change. Keep status and apply on the
same plan. Check [AGENTS.md](AGENTS.md) for the code map and compatibility conventions.

Never run apply, sync or install against your real home during development.

## Issues

Include `dot version` and `dot help` output. Remove anything private first.

## License

By contributing, you agree that your contributions are licensed under the MIT License.
