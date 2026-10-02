# Performance budgets for benchmarks/bench.sh: portable ceilings, not aspirational
# targets. They are intentionally well above healthy measurements so shared CI
# noise does not fail a change; crossing one means a regression deserves an
# explicit investigation and budget change in the same review. Milliseconds,
# except budget_binary_size_bytes.
budget_binary_size_bytes=16000000
budget_cold_start_ms=100
budget_status_ms=2000
budget_apply_ms=2000
budget_sync_ms=5000
