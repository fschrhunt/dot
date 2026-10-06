#!/bin/sh
# One repository entry point. Local checks and CI run the same commands, so no environment has
# a private definition of "green".
set -eu
cd "$(dirname "$0")"

# Print supported targets and their argument contracts.
usage() {
    cat <<'EOF'
usage: ./x [command] [args...]
  check              Default; Go formatting, vet, build and tests, shellcheck and repository guards
  fmt [--check]      Format Go sources, or check without writing
  lint               Run go vet
  test [args...]     Forward arguments to go test (default: ./...)
  build [args...]    Forward arguments to go build (default: ./...)
  shell | guard      Shellcheck or repository guards
  smoke              End-to-end checks in a temporary home
  audit | links      Network vulnerability audit or external link checks
  bench              Enforce benchmark budgets
  release-check [tag] Check, build release targets and run smoke checks
  hooks              Install the existing Git hooks
  help | --help | -h Show this help
EOF
}

# Reject invalid arguments before running any command that could write files.
invalid() { usage >&2; exit 2; }

command=${1-check}
if [ "$#" -gt 0 ]; then shift; fi

case "$command" in
    check|lint|shell|smoke|guard|audit|bench|hooks|links) [ "$#" -eq 0 ] || invalid ;;
    fmt)
        [ "$#" -eq 0 ] || { [ "$#" -eq 1 ] && [ "$1" = "--check" ]; } || invalid
        ;;
    help|--help|-h) [ "$#" -eq 0 ] || invalid; usage; exit 0 ;;
    release-check) [ "$#" -le 1 ] || invalid ;;
esac

case "$command" in
    # CI groups these commands into jobs; a local check uses the same ones.
    check)
        ./x fmt --check
        ./x lint
        ./x build
        ./x test
        ./x shell
        ./x guard
        ;;
    fmt)
        if [ "${1:-}" = "--check" ]; then
            unformatted=$(gofmt -l .)
            if [ -n "$unformatted" ]; then
                printf 'format these with: gofmt -w %s\n' "$unformatted" >&2
                exit 1
            fi
        else
            gofmt -l -w .
        fi
        ;;
    lint) go vet ./... ;;
    build)
        if [ "$#" -eq 0 ]; then set -- ./...; fi
        go build "$@"
        ;;
    test)
        if [ "$#" -eq 0 ]; then set -- ./...; fi
        go test "$@"
        ;;
    shell)
        command -v shellcheck >/dev/null 2>&1 || { echo "shell: shellcheck is not on PATH" >&2; exit 2; }
        shellcheck install.sh x scripts/*.sh .githooks/pre-commit
        ;;
    smoke) scripts/smoke.sh ;;
    guard) scripts/guard.sh ;;
    audit)
        # Needs the network for the vulnerability database; CI and releases run it, not pre-commit.
        go run golang.org/x/vuln/cmd/govulncheck@v1.8.0 ./...
        ;;
    bench)
        sh benchmarks/bench.sh --check
        ;;
    hooks)
        git config core.hooksPath .githooks
        printf 'git hooks installed: pre-commit runs gofmt and go vet.\n'
        ;;
    links)
        command -v lychee >/dev/null 2>&1 || { echo "links: lychee not on PATH (see https://lychee.cli)" >&2; exit 2; }
        # shellcheck disable=SC2046
        lychee --config .lychee.toml $(find README.md AGENTS.md docs -name '*.md')
        ;;
    release-check)
        ./x check
        tag=${1:-v0.0.0-check}
        for os in linux darwin; do
            for arch in amd64 arm64; do
                GOOS="$os" GOARCH="$arch" go build -trimpath -ldflags "-s -w -X main.version=$tag" -o /dev/null ./cmd/dot
            done
        done
        scripts/smoke.sh
        printf 'release check passed for %s\n' "$tag"
        ;;
    *) invalid ;;
esac
