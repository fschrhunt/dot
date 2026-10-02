# Configuration

The setup's folders are its configuration, and `~/.dot/dot.toml` holds the rest. Set `DOT_HOME`
for a different setup folder. The examples below are separate snippets. Merge them into one
table per name.

## The folders

```text
~/.dot/
  home/.gitconfig              written to ~/.gitconfig
  home/.config/zsh/aliases.zsh written to ~/.config/zsh/aliases.zsh
  home/.note.tmpl              a template, written to ~/.note
  agents/instructions.md       written under each agent's name; see agents.md
  agents/skills/review/        written to each agent's skills folder
```

Each file under `home/` is managed on its own: dot owns that file and nothing else in the
folder around it. Each child of a folder under `agents/` is a unit, and a file created inside
one of its copies is taken as part of it. `dot add` and `dot forget` maintain both folders.

`dot.toml` is optional. With no `dot.toml`, a setup that has either folder is a version 2 setup
with every default.

## version

```toml
version = 2
```

Defaults to 1. Must be a whole number. A newer version asks you to update dot.

Version 2 turns on the folders above and two-way sync. A version 1 setup uses only the
mappings in `dot.toml` and is applied one way; nothing here changes for it.

## exclude

```toml
exclude = [".DS_Store", "*.tmp"]
```

Defaults to `[]`. For directory mappings, globs match each path segment.
Excluded files are never copied or deleted, even if dot wrote them before.
Excludes do not suppress a mapping whose source is a single file.
For compatibility, patterns matching `.` (such as `.*`) also skip every source descendant.
Use specific names such as `.DS_Store` to avoid that quirk.

## sync.push

```toml
[sync]
push = true
```

Defaults to true in a version 2 setup and false in a version 1 setup. With an upstream, sync
pushes local commits when ahead.

## sync.take

```toml
[sync]
take = false
```

Defaults to true in a version 2 setup. `dot sync` then takes edits made to live files into the
setup; see [sync](sync.md). Set it to false, for every machine or in a
[`[sync.machine.<name>]`](#syncmachinex) table for one, to make that machine one-way: the setup
is applied to it, and an edited live file is left alone until you run `dot take`.

`push` also defaults to true in a version 2 setup.

## sync.every

```toml
[sync]
every = "1m"
```

How often the timer runs `dot sync`. Defaults to `"15m"`. Write whole seconds, minutes or
hours, alone or combined: `"90s"`, `"5m"`, `"1h"`, `"1h30m"`.

`dot timer` reads this when it writes the timer, so run `dot timer` again on each machine
after changing it:

```sh
dot timer
# Installed: dot sync runs every 1 minute on laptop.
```

## sync.after_boot

```toml
[sync]
after_boot = "30s"
```

How long after boot the Linux timer waits before its first run. Defaults to `"2m"`; `"0s"` runs at boot.
Read by `dot timer`, like `every`. macOS runs the first sync when the agent loads.

## sync.timeout and sync.connect_timeout

```toml
[sync]
timeout = "3m"
connect_timeout = "20s"
```

`timeout` stops each git command, and each mapping's `run` command, that takes longer.
It defaults to `"60s"`. `connect_timeout` is how long ssh may take to connect, and defaults
to `"5s"`. Raise them for a slow link or a large setup.

## sync.machine.X

```toml
[sync]
every = "15m"

[sync.machine.server]
every = "1m"
push = true
```

The `server` machine syncs every minute and pushes. Other machines keep `[sync]`.
A machine's table takes the same keys as `[sync]`. Every machine's table is checked on
every machine.

## agent.X and only

```toml
[agent.myagent]
home = "~/.myagent"
instructions = "RULES.md"

[only]
"agents/skills/browser" = { agents = ["claude", "codex"] }
"home/.config/hypr" = { machines = ["desktop"] }
```

`[agent.<name>]` adds an agent or changes a built-in one; see [agents](agents.md). `[only]`
limits a path in the setup, and everything under it, to some agents or some machines.

## values

```toml
[values]
workspace_root = "~/Code"
port = 8080
ratio = 1.5
```

Strings and numbers become placeholder text. Booleans are rejected.
Values are not recursively rendered.

## machine.X

```toml
[values]
workspace_root = "~/Code"
[machine.server]
workspace_root = "~/src"
```

The `server` machine overrides the base value. Other machines keep `~/Code`.
The name is `hostname -s`, or `DOT_MACHINE`. An empty machine table is allowed.
All known machines are validated before commands use the config.

## templates

A name ending in `.tmpl` under `home/` or `agents/` is a template without any line here. This
table is for a template that needs an explicit mapping.

```toml
[templates]
"instructions.md" = ["~/.claude/CLAUDE.md", "~/.codex/AGENTS.md"]
```

`instructions.md` might contain:

```text
Machine: {{machine}}
Code: {{workspace_root}}
```

Templates must be text files. Every `{{name}}` must be defined on every applicable machine.
A template accepts the same string, array and table forms as a file mapping.
A source symlink is validated as a template but copied as a symlink, as in the Python version.
`dot take` refuses template destinations and shows a diff instead.

## files: one destination

`home/` covers most files without any line here. Use `[files]` for what a folder cannot say: a
destination outside your home folder, one that depends on a value, or a folder that dot should
own whole.

```toml
[files]
"gitconfig" = "~/.gitconfig"
```

Sources are relative to the setup (absolute sources also work).
Destinations must be absolute or begin with `~`. Files copy verbatim.
Symlinks copy as symlinks. New files take source permissions; existing files keep theirs.

## files: several destinations

```toml
[files]
"skills/review" = ["~/.claude/skills/review", "~/.codex/skills/review"]
```

A directory copies its tree to each destination. The default is an exact mirror.

## files: table form and to

```toml
[files."zsh"]
to = "~/.config/zsh"
mirror = false
exclude = ["local.zsh"]
machines = ["laptop", "server"]
```

`to` is required. It accepts a nonempty string array as well, and the table also takes [`run`](#run):

```toml
[files]
"skills" = { to = ["~/.claude/skills", "~/.codex/skills"], mirror = true }
```

Unknown mapping options are errors. Unknown top-level keys are ignored for compatibility.

## run

```toml
[files]
"tmux.conf" = { to = "~/.tmux.conf", run = "tmux source-file ~/.tmux.conf" }
```

A command to run after apply or sync changes anything in the mapping: once, after all the
files are written, and never when nothing changed. Mappings run in the order dot.toml lists them.
The command runs through `sh` in your home folder with `DOT_MACHINE` set, and stops at
[`sync.timeout`](#synctimeout-and-syncconnect_timeout).

```sh
dot status
# ~ changed     ~/.tmux.conf
# > run         tmux source-file ~/.tmux.conf
```

A command that fails is reported as `run failed`, and apply or sync exits 1. The files stay
written, so the command does not run again until the mapping next changes.
The setup can run commands on every machine that syncs it, so keep its remote private.

## mirror

```toml
[files]
"config" = { to = "~/.config/mytool", mirror = false }
```

Defaults to true. A mirror removes extra files and prunes empty folders.
With false, extras stay and status labels them `? extra`.
Files previously written by dot can still be removed if dropped from the source.

## mapping exclude

```toml
[files]
"zsh" = { to = "~/.config/zsh", exclude = ["local.zsh", "cache"] }
```

These patterns add to the global excludes. An excluded directory is skipped as a whole.

## machines

```toml
[files]
"server.conf" = { to = "~/.config/app.conf", machines = ["server"] }
```

Defaults to every machine. An empty list disables the mapping.
The listed names are validated even if they have no `[machine.X]` table.

## {{machine}} and other placeholders

```toml
[machine.server]
[files]
"config.{{machine}}.json" = "~/.config/app/config.json"
"guide.md" = "{{workspace_root}}/guide.md"
```

With the values above, provide both `config.server.json` and a source for the current machine.
`machine` is always defined and overrides a user value with that name.
Whitespace inside braces is allowed: `{{ machine }}`.
Placeholders work in sources, destinations and template text.

Two mappings cannot write the same destination, even through symlinked parents.
One destination cannot be inside another. A managed folder replaced by a symlink is treated
as edited. dot never writes through that replacement link.
