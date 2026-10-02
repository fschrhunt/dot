#!/bin/sh
# guard: the promises dot makes to its users, checked on every change. Each check pins one
# invariant from AGENTS.md; a change that moves a promise must move this script in the same
# diff, where the review can see it.
set -eu
cd "$(dirname "$0")/.."
fail=0

bad() { printf 'guard: FAIL: %s\n' "$1" >&2; fail=1; }

# Production Go sources only: a promise that tests already need bent lives elsewhere.
prod_go() { find cmd internal -name '*.go' -not -name '*_test.go'; }

# 1. Sync never merges. The pull must keep being ff-only or rebase; a merge bakes in a
#    resolution dot cannot explain afterward.
if prod_go | xargs grep -nH '"merge"' | grep -q .; then
    bad "a merge command appears in dot code; a mapped mistake never merges"
fi

# 2. Sync never takes a deletion and never replaces a path it did not look at. Those two
#    invariants live in plan; the greps exist so a careless rename cannot slip past.
grep -q '"delete"' internal/plan/plan.go || bad "plan lost the delete action"
grep -q 'ViaLink' internal/plan/plan.go || bad "ViaLink is gone; a symlinked folder may be replaced"

# 3. Dot never talks to the network; git, through the user's transport, does. Importing a
#    dialer here would change who and what sees user data.
if prod_go | xargs grep -hE '"net"|"net/http"|"crypto/tls"' | grep -q .; then
    bad "dot imports a network package; dot itself never speaks to the internet"
fi

# 4. The credential gate runs in front of every commit and every init-time copy.
if ! prod_go | xargs grep -l 'plan.Secret' | grep -q 'sync'; then
    bad "sync no longer gates commits on plan.Secret"
fi
grep -q 'plan.Secret' internal/app/manage.go || bad "dot add no longer gates on plan.Secret"

# 5. protectState runs before take and pull; committed bookkeeping would otherwise own the
#    repository's history of every machine.
grep -q 'protectState()' internal/sync/sync.go || bad "sync no longer protects .state/ from commits"

# 6. A timer firing is not a Go sleep.
if prod_go | xargs grep -nH 'time.Sleep' | grep -q .; then
    bad "a time.Sleep in dot code; settle by file signatures or timeouts"
fi

# 7. On a bad written.json the timer must say which file and what to do, not print a raw
#    parser error.
grep -q 'cannot parse' internal/apply/apply.go || bad "apply no longer names written.json on bad JSON"

# 8. An unfinished merge marker makes the docs a contradiction.
if grep -rnE '^(<{7}|={7}|>{7}|\|{7})' README.md CHANGELOG.md AGENTS.md docs 2>/dev/null | grep -q .; then
    bad "a merge marker in the docs"
fi

# 9. CI must be trustworthy of itself: actions are pinned by SHA, never a moving tag.
for wf in .github/workflows/*.yml; do
    if grep -n 'uses:' "$wf" | grep -vE '@[0-9a-f]{40}' | grep -q .; then
        bad "$wf has an action not pinned by SHA"
    fi
done

# 10. CODEOWNERS must only name paths that still exist — otherwise ownership silently
#    stops watching the area it claimed after a rename.
for pattern in $(awk '!/^#/ && NF { print $1 }' .github/CODEOWNERS); do
    case "$pattern" in
        \*|*\**|*\?*|*\[*) continue ;;
    esac
    if [ ! -e "${pattern#/}" ]; then
        bad "CODEOWNERS names a missing path: $pattern"
    fi
done

if [ "$fail" -eq 0 ]; then
    echo 'guard: all checks passed'
fi
exit "$fail"
