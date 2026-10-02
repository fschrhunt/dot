# dot: notes for agents

## Commands

- Test: `go test ./...` (offline after the module is fetched; temp HOME, DOT_HOME and local bare repos).
- One test: `go test ./internal/sync -run '^TestSyncRefusesUncommittedChanges$'`.
- Checks: `go build ./...`, `go vet ./...`, and `gofmt -l .` (must print nothing).
- Build the CLI: `go build -o dot ./cmd/dot`. Try the sample with `DOT_HOME=example ./dot help`.
  Never run `apply`, `sync` or `timer` against a real home while developing;
  point HOME and DOT_HOME at temp folders.

## Code map

- `cmd/dot/main.go`: argument parsing, dispatch, version and exit handling.
- `internal/config/`: TOML loading, values, rendering, resolution and validation for every machine;
  `layout.go` holds the agents table and turns `home/` and `agents/` into mappings.
- `internal/plan/`: desired paths, the shared ordered plan (takes, deletions, writes), what may be
  taken, plan output, unified diffs and carrying an edit back into a template.
- `internal/apply/`: executing the plan, edited-in-place protection, pruning, written.json and mappings' run commands.
- `internal/setup/`: captured setup paths, user errors, traversal, signatures and atomic writes.
- `internal/sync/`: locking, the ssh command and timeouts for git, taking and committing, pull
  (rebase for two-way, ff-only for one-way), push, apply, conflict recording and sync logs.
- `internal/schedule/`: systemd user timer and launchd agent installation and removal.
- `internal/app/`: status, apply, take, init and help command handlers; `manage.go` has add, forget
  and agents.
- `internal/testutil/`: temporary homes, handler capture and local git fixtures for tests.
- `example.go` and `example/`: the embedded starter setup, a commented version 2 `dot.toml`.
- `docs/`: user help with worked examples.
- `assets/`: the wordmark and logo SVGs in black and white; see `assets/README.md`.

## Conventions

- Fewest moving parts: standard library plus one TOML parser. No abstractions the change does not need.
- Packages and exported functions have short purpose-and-contract doc comments. No line-by-line comments.
- `dot status` must print exactly what `dot sync` does, and `dot apply -n` what `dot apply` does:
  change the shared plan in `internal/plan`, never one command alone.
- A version 1 setup must behave exactly as it did: two-way sync and the folders are version 2 only.
- Sync never merges, never takes a deletion, and never replaces a path it did not just look at.
  It never commits a line shaped like a credential or a setup that does not load, and never
  removes a file dot did not write.
- Preserve Python behavior and the .state formats for existing setups, including known quirks.
- Errors are one line per problem, prefixed `dot:`, and name what to fix.
- One focused test per protected behavior, next to the package it pins. No smoke tests.
- Update every comment and doc affected by a code change.
- A CHANGELOG.md entry under Unreleased for every user-visible change.
