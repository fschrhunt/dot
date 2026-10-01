<div align="center">

# dot

Your setup, the same on every machine.

[![CI](https://github.com/fschrhunt/dot/actions/workflows/ci.yml/badge.svg)](https://github.com/fschrhunt/dot/actions/workflows/ci.yml)

</div>

You keep your setup in one private git folder, `~/.dot`: agent instructions, skills, shell and
editor config, small tools. Every machine runs `dot sync` on a timer. It fast-forwards that folder
from a git remote you choose, pushes your own new commits back when you turn that on, and copies
each file to every place your config lists. No server, no account. dot is one Python file that
needs only Python 3.11+ and git.

## Install

```sh
git clone https://github.com/fschrhunt/dot ~/.local/share/dot
mkdir -p ~/.local/bin
ln -s ~/.local/share/dot/dot ~/.local/bin/dot
```

`~/.local/bin` must be on your `PATH`. If `command -v dot` prints nothing, add
`export PATH="$HOME/.local/bin:$PATH"` to your shell profile and open a new shell. dot runs under
whatever `python3` it starts with; if that is older than 3.11, it looks for a newer one on `PATH` and
in the usual Homebrew folders, so a bare, non-login ssh works too.

## Start

```sh
dot init              # creates ~/.dot from the example (or: dot init <remote> to clone yours)
$EDITOR ~/.dot/dot.toml
dot                   # shows what apply would do
dot apply             # writes it
dot install           # runs dot sync every 15 minutes on this machine
```

Commit your changes in `~/.dot`; sync refuses to run while tracked files have uncommitted changes.

## Sync between machines

The remote is any git remote. A private hosted repo works, and so does a bare repo on a machine
you keep running:

```sh
ssh server git init --bare dot.git
cd ~/.dot && git remote add origin server:dot.git && git push -u origin HEAD
```

On each other machine, `dot init server:dot.git`, then `dot apply` and `dot install`. Sync only
fast-forwards: it never merges, rebases or resets. If two machines diverge, fix it by hand in
`~/.dot` with git.

## Configuration

`~/.dot/dot.toml` (set `DOT_HOME` to use another folder):

```toml
version = 1                          # dot refuses a newer version: "update dot"
exclude = [".DS_Store", "*.tmp"]     # never copied, never deleted (glob, matched on each path segment)

[sync]
push = true                          # push local commits the remote lacks (fast-forward only)

[values]
workspace_root = "~/Code"

[machine.server]                     # overrides for one machine
workspace_root = "~/src"

[templates]                          # source = destination or [destinations]; rendered, then written
"instructions.md" = ["~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"]

[files]                              # copied verbatim; a source may fan out to many destinations
"skills/review" = ["~/.claude/skills/review", "~/.codex/skills/review"]
"code/README.md" = "{{workspace_root}}/README.md"
"opencode/config.{{machine}}.jsonc" = "~/.config/opencode/opencode.jsonc"   # a per-machine source

[files."zsh"]                        # the table form, for per-mapping options
to = "~/.config/zsh"
mirror = false                       # keep files dot did not write (default true: exact mirror)
exclude = ["local.zsh"]              # extra patterns for this mapping only
machines = ["laptop", "server"]      # only on these machines (default: every machine)
```

- `{{name}}` works in templates, destinations and sources. `machine` is always defined: the
  output of `hostname -s`, or `DOT_MACHINE`.
- Every command first checks the config for every machine: wrong types (`mirror = "false"` is an
  error, not true), undefined names, missing sources, two mappings writing one place (also through
  a symlinked folder), and one destination inside another.
- A file you edit in place is never overwritten or deleted without `--force`. Copy it back with
  `dot take`, or let dot replace it with `dot apply --force`. A managed folder you replace with a
  symlink counts as edited too, and dot never writes or deletes through it.
- A new file or folder takes its source's permissions; an existing one keeps its own.
- Each machine keeps its own state in `~/.dot/.state/` (ignored by git): what dot wrote, and the
  sync log.

## Commands

| Command | What it does |
|---|---|
| `dot`, `dot status [path]` | The last sync result, then the plan: `+` new, `~` changed, `-` removed, `!` edited here, `?` extra. With a path, a diff. Exits 1 when something would change. |
| `dot apply [-n] [--force]` | Writes the plan. `-n` only prints it. `--force` overwrites files edited here. |
| `dot sync` | Pulls (fast-forward only), pushes when ahead and `push` is on, then applies. Quiet; logs one line to `.state/sync.log`. |
| `dot take <path>` | Copies a live file or folder back to its source in `~/.dot`. Does not commit. |
| `dot init [remote]` | Creates `~/.dot` from the example, or clones your remote. |
| `dot install [--remove]` | Adds (or removes) the 15-minute timer: a systemd user timer on Linux, a LaunchAgent on macOS. The timer keeps your `PATH`, `DOT_HOME` and `DOT_MACHINE`. |
| `dot help` | Usage, then this machine's values and mappings. |

## License

MIT
