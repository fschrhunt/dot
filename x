#!/bin/sh
# One repository entry point. Local checks and CI run the same commands, so no environment has
# a private definition of "green".
set -eu
cd "$(dirname "$0")"

usage() {
    echo "usage: ./x [check|fmt|lint|build|test|smoke|guard|hooks|links|release-check]" >&2
    exit 2
}

command=${1:-check}
if [ "$#" -gt 0 ]; then shift; fi

case "$command" in
    # CI groups these commands into jobs; a local check uses the same ones.
    check)
        ./x fmt --check
        ./x lint
        ./x build
        ./x test
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
    build) go build ./... ;;
    test) go test ./... "$@" ;;
    smoke) scripts/smoke.sh ;;
    guard) scripts/guard.sh ;;
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
    *) usage ;;
esac
