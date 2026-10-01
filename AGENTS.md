# dot: notes for agents

## Commands

- Test: `python3 test.py` (offline; temp HOME, DOT_HOME and a local bare repo).
- One test: `python3 test.py Dot.test_sync_refuses_uncommitted_changes`.
- Try it against the sample: `DOT_HOME=example python3 dot help`. Never run `apply`, `sync` or
  `install` against a real home folder while developing; point HOME and DOT_HOME at a temp folder.

## Code map

- `dot`: the whole program, top to bottom: the launcher (`find_python`), config (`load`, `resolve`,
  `validate`, `render`), the plan (`wants`, `plan`), carrying it out (`write`, `prune`, `apply`),
  commands (`status`, `take`, `sync`, `init`, `install`, `help_`), and `main`.
- `test.py`: one unittest per protected behavior.
- `example/`: the setup `dot init` copies; it must stay valid for any machine name.

## Conventions

- Fewest moving parts: one file, standard library only, no abstraction the change does not need.
- Every function and the module say their purpose and contract in a short docstring; no
  line-by-line comments.
- `status` must print exactly what `apply` does: change the plan, never one of them alone.
- Errors are one line, prefixed `dot:`, and name what to fix.
- One test per behavior change; no smoke tests.
- A CHANGELOG.md entry under Unreleased for every user-visible change.
