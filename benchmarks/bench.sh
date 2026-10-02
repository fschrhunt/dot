#!/bin/sh
# bench: dot's benchmark suite — the numbers a 15-minute timer depends on.
# shellcheck disable=SC2154 # the budget_* variables land through benchmarks/budgets.sh below
# A plain run measures and writes a timestamped report into benchmarks/results/.
# --check enforces benchmarks/budgets.sh without writing anything, for CI: it is
# a stable regression alarm, not a microbenchmark contest. Any other argument
# is refused. Numbers are only comparable within one machine.
set -eu
cd "$(dirname "$0")/.."

check=0
if [ $# -gt 0 ]; then
  [ "$1" = "--check" ] && [ $# -eq 1 ] || { echo "usage: benchmarks/bench.sh [--check]" >&2; exit 2; }
  check=1
fi
command -v perl >/dev/null 2>&1 || { echo "bench: needs perl for millisecond timing" >&2; exit 2; }

# A throwaway home with 2000 managed files over 200 folders, one shared skill,
# and one agent installed: the read set a real sync walks every 15 minutes.
files=2000
work=$(mktemp -d "${TMPDIR:-/tmp}/dot-bench.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM

# Build before overriding HOME, so the Go module cache resolves to the real one: a cold
# module cache inside the throwaway home would make the script network-dependent.
bin="$work/dot"
go build -trimpath -o "$bin" ./cmd/dot

export HOME="$work/home" DOT_HOME="$work/home/.dot" DOT_MACHINE=bench
export GIT_CONFIG_NOSYSTEM=1 GIT_CONFIG_GLOBAL=/dev/null
git() { command git -c user.name=bench -c user.email=bench@example.com -c init.defaultBranch=main "$@"; }

mkdir -p "$DOT_HOME/home/.config" "$DOT_HOME/agents/skills/demo" "$HOME/.claude"
printf 'version = 2\n' > "$DOT_HOME/dot.toml"
printf '.state/\n' > "$DOT_HOME/.gitignore"
printf 'shared instructions\n' > "$DOT_HOME/agents/instructions"
printf 'name: demo\n' > "$DOT_HOME/agents/skills/demo/SKILL.md"
i=0
while [ "$i" -lt "$files" ]; do
  d="$DOT_HOME/home/.config/d$((i % 200))"
  [ -d "$d" ] || mkdir -p "$d"
  printf 'setting %s = %s\nsecond line\nthird line\n' "$i" "$i" > "$d/f$i.conf"
  i=$((i + 1))
done
git -C "$DOT_HOME" init -q
git -C "$DOT_HOME" add -A
git -C "$DOT_HOME" commit -qm setup
"$bin" apply > /dev/null

now_ms() { perl -MTime::HiRes=time -e 'printf "%d\n", time * 1000'; }
median() { # median FILE holding one integer per line; the count stays odd
  sort -n "$1" | sed -n "$(( ($(wc -l < "$1") + 1) / 2 ))p"
}
timed() { # timed RUNS FILE COMMAND...: append one ms sample per run
  runs=$1; samples=$2; shift 2
  i=0
  while [ "$i" -lt "$runs" ]; do
    start=$(now_ms)
    "$@" > /dev/null 2>&1
    printf '%s\n' "$(( $(now_ms) - start ))" >> "$samples"
    i=$((i + 1))
  done
  median "$samples"
}

size=$(wc -c < "$bin")
cold=$(timed 21 "$work/cold" "$bin" --version)
status=$(timed 5 "$work/status" "$bin" status)
apply=$(timed 5 "$work/apply" "$bin" apply)
sync=$(timed 5 "$work/sync" "$bin" sync)

commit=$(git rev-parse --short HEAD)
version=$("$bin" --version)
machine="$(uname -m) · $(uname -s) $(uname -r)"
stamp=$(date +%Y-%m-%d_%H-%M)
report=$(cat <<EOF
date:            $stamp
version:         $version ($commit)
machine:         $machine
binary size:     $size bytes
cold start:      $cold ms   (dot --version, median of 21)
status:          $status ms   (dot status on $files managed files, median of 5)
apply:           $apply ms   (dot apply with nothing to do, median of 5)
sync:            $sync ms   (dot sync with no upstream, median of 5)
EOF
)
printf '%s\n' "$report"

if [ "$check" = 1 ]; then
  # shellcheck disable=SC1091 # budgets.sh is the budgets, beside this script
  . ./benchmarks/budgets.sh
  failed=0
  over() { # over NAME VALUE LIMIT
    if [ "$2" -gt "$3" ]; then echo "  $1: $2 > $3" >&2; failed=1; fi
  }
  over "binary size" "$size" "$budget_binary_size_bytes"
  over "cold start" "$cold" "$budget_cold_start_ms"
  over "status" "$status" "$budget_status_ms"
  over "apply" "$apply" "$budget_apply_ms"
  over "sync" "$sync" "$budget_sync_ms"
  if [ "$failed" = 1 ]; then
    echo "performance budget exceeded" >&2
    exit 1
  fi
  echo "performance budgets: passed"
  exit 0
fi

out="benchmarks/results/${stamp}_${commit}.txt"
printf '%s\n' "$report" > "$out"
echo "written: $out"
