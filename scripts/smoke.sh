#!/bin/sh
# smoke: exercise the installed binary end to end, in a throwaway HOME with a throwaway remote.
# CI runs it after `go build`; it catches the integration failures a unit test cannot:
# generated paths, git'd identity, .state hygiene, and a take flowing back into the setup.
set -eu
cd "$(dirname "$0")/.."

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

# Build before overriding HOME, so the Go module cache resolves to the real one: a cold
# module cache inside the throwaway home would make the script network-dependent.
bin="$tmp/dot"
go build -trimpath -o "$bin" ./cmd/dot

export HOME="$tmp/home"
export DOT_HOME="$tmp/home/.dot"
export DOT_MACHINE=laptop
export GIT_CONFIG_NOSYSTEM=1
export GIT_CONFIG_GLOBAL=/dev/null
export GIT_AUTHOR_NAME=smoke
export GIT_AUTHOR_EMAIL=smoke@example.com
export GIT_COMMITTER_NAME=smoke
export GIT_COMMITTER_EMAIL=smoke@example.com

fail() { echo "smoke: FAIL: $*" >&2; exit 1; }

expect() { # expect <name> <grep-pattern> <actual-output>
    case "$3" in
        *"$2"*) ;;
        *) fail "$1: expected output to contain '$2', got:"$(printf '\n%s\n' "$3");;
    esac
}

mkdir -p "$HOME/.claude/skills/review" "$HOME/.codex" "$HOME/.config"
"$bin" init > /dev/null
printf '[user]\n\tname = Smoke\n' > "$HOME/.config/gitconfig"
printf '# Rules\nBe brief.\n' > "$HOME/.claude/CLAUDE.md"
printf 'name: review\n' > "$HOME/.claude/skills/review/SKILL.md"

"$bin" add "$HOME/.config/gitconfig" "$HOME/.claude/CLAUDE.md" "$HOME/.claude/skills/review" > /dev/null || fail 'dot add failed'

[ -f "$HOME/.codex/AGENTS.md" ] || fail "AGENTS.md for the second agent missing after add"
[ -f "$HOME/.codex/skills/review/SKILL.md" ] || fail "skill never reached codex"

status_out=$("$bin" status || true)
expect "status settled after add" 'up to date' "$status_out"

expect "help explains the setup" 'home/.config/gitconfig' "$("$bin" help)"

"$bin" sync > /dev/null || fail "first sync failed"
expect "sync works upstream-less" "no upstream" "$(tail -1 "$DOT_HOME/.state/last")"
[ -f "$HOME/.codex/AGENTS.md" ] || fail "~/.codex/AGENTS.md was never written"
[ -f "$HOME/.codex/skills/review/SKILL.md" ] || fail "the skill never reached codex"
tracked=$(git -C "$DOT_HOME" ls-files | grep -c '^\.state/' || true)
[ "$tracked" = 0 ] || fail "dot committed its own .state"

printf '# Rules\nBe brief and kind.\n' > "$HOME/.claude/CLAUDE.md"
printf '# Rules\nBe brief and kind.\n' > "$HOME/.codex/AGENTS.md"
out=$("$bin" sync 2>&1) || fail "sync after live edit failed: $out"
[ "$(grep -c 'kind' "$DOT_HOME/agents/instructions.md")" = "1" ] \
    || fail "the taken edit did not reach the setup: $(cat "$DOT_HOME/agents/instructions.md")"
log_msg=$(git -C "$DOT_HOME" log -1 --format=%s)
expect "take committed under the machine name" 'laptop: ~/.claude/CLAUDE.md' "$log_msg"

out=$("$bin" log "$HOME/.claude/CLAUDE.md")
expect "dot log shows the take" 'laptop:' "$out"

out=$("$bin" forget "$HOME/.config/gitconfig")
expect "forget stops managing but keeps the file" "it stays where it is" "$out"
[ -f "$HOME/.config/gitconfig" ] || fail "forget removed the live file"

"$bin" status > "$tmp/status" || status_rc=$?
status_rc=${status_rc:-0}
[ "$status_rc" = 0 ] || fail "status is 1 after steady state: $(cat "$tmp/status")"
expect "status settled" "up to date" "$(cat "$tmp/status")"

echo 'smoke: all checks passed'
