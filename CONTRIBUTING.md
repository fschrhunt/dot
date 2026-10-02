# Contributing

Smallness is the point. dot is a Go binary with one TOML dependency and git at runtime.
An extra package, dependency or option has to earn its place. Removing complexity is welcome.

## Setup and tests

Use Go 1.26 or newer:

```sh
git clone https://github.com/fschrhunt/dot
cd dot
./x hooks        # run gofmt and go vet on staged Go files before each commit
./x check        # the whole gate: fmt, vet, build, tests, guard
```

`./x` is the one entry point: CI runs the same commands, so local and remote never
disagree about green. `./x smoke` exercises the built binary end to end in a throwaway
home; `./x release-check` also builds every release target. `./x links` checks external
links (needs [lychee](https://lychee.cli)); CI runs it weekly.

Fetch the module once with `go mod download`. Tests then run offline in temp homes and local
bare git repos. Add one focused test per behavior you change. `scripts/guard.sh` pins the
promises in AGENTS.md that a change must not move silently; a PR that moves one changes the
script in the same diff. Keep status and apply on the same plan. Check [AGENTS.md](AGENTS.md)
for the code map and compatibility conventions.

Never run apply, sync or timer against your real home during development.

## Issues

Include `dot version` and `dot help` output. Remove anything private first.

## License

By contributing, you agree that your contributions are licensed under the MIT License.
