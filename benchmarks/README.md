# Benchmarks

The numbers the 15-minute timer depends on: a small binary and a fast steady state.

```sh
./x bench                       # enforce CI's generous regression budgets
sh benchmarks/bench.sh          # record a local result for comparison
```

Each run writes a timestamped report into [results/](results/) — commit the
ones worth keeping (a release, a big refactor) so regressions have a paper
trail. Numbers are only comparable within one machine.

`benchmarks/budgets.sh` contains portable ceilings, not aspirational
targets. They are intentionally well above healthy measurements so shared CI
noise does not fail a change; crossing one means a regression deserves an
explicit investigation and budget change in the same review.

Measured today: binary size, cold start (`dot --version`, median of 21),
and the steady state on 2000 managed files over 200 folders with one shared
skill — `dot status`, a `dot apply` with nothing to do, and a `dot sync`
with no upstream (median of 5 each).
