# Configuration

Edit `~/.dot/dot.toml`. Set `DOT_HOME` for a different setup folder.
The examples below are separate snippets. Merge them into one table per name.

## version

```toml
version = 1
```

Defaults to 1. Must be a whole number. A newer version asks you to update dot.

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

Defaults to false. With an upstream, sync pushes local commits when ahead.
It never creates commits.

## sync.every

```toml
[sync]
every = "1m"
```

How often the timer runs `dot sync`. Defaults to `"15m"`. Write whole seconds, minutes or
hours, alone or combined: `"90s"`, `"5m"`, `"1h"`, `"1h30m"`.

`dot install` reads this when it writes the timer, so run `dot install` again on each machine
after changing it:

```sh
dot install
# Installed: dot sync runs every 1 minute on laptop.
```

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

`to` is required. It accepts a nonempty string array as well:

```toml
[files]
"skills" = { to = ["~/.claude/skills", "~/.codex/skills"], mirror = true }
```

Unknown mapping options are errors. Unknown top-level keys are ignored for compatibility.

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
