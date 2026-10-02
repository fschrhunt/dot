# dot 2: design

Status: proposal, under review. This replaces the mapping-first design. Nothing here is built yet
except where "today" says so.

## What dot is for

dot is the dotfiles manager. It keeps the files that configure your tools the same on every
machine, and it treats the coding agents on those machines as one audience: the same
instructions and skills reach Claude Code, Codex, opencode and the rest, although each agent
gives those files a different name in a different folder.

Two things set it apart from chezmoi, GNU Stow, yadm and dotbot:

1. **You edit files where they live.** There is no source folder to remember. dot notices the
   edit, records it, and carries it to your other machines.
2. **One file can have several names.** `~/.claude/CLAUDE.md` and `~/.codex/AGENTS.md` are one
   file to dot. An edit to either reaches the other.

Anyone can sync `~/.codex` between machines, and anyone can sync `~/.claude`. Nothing syncs
them with each other. That is the gap dot takes.

## What changes from dot today

| Today | dot 2 |
| --- | --- |
| `dot.toml` lists every source and its destinations | The setup's folders say where files go; `dot.toml` is optional |
| One-way: the setup is applied to the machine | Two-way: an edit on the machine is taken into the setup |
| Never commits; you commit and `dot take` by hand | dot commits and pushes what it takes |
| `git pull --ff-only`; divergence is yours to fix | `git pull --rebase`; a conflict is reported, never forced |
| An edited live file is protected until you act | It is taken, unless the setup changed too |
| A list of destinations per skill, repeated | A table of agents, built in and yours to extend |

What stays: files are copied, not linked; `status` prints exactly what `sync` and `apply` will
do; a timer runs `dot sync`; an explicit `[files]` mapping still works for anything unusual.

## The setup

The setup is still a private git repository at `~/.dot` (or `DOT_HOME`). Its layout is the
configuration:

```text
~/.dot/
  home/                     mirrors your home folder
    .gitconfig              -> ~/.gitconfig
    .config/zsh/…           -> ~/.config/zsh/…
    .claude/keybindings.json-> ~/.claude/keybindings.json   (one agent only)
  agents/                   shared by every agent that has a place for it
    instructions.md         -> ~/.claude/CLAUDE.md, ~/.codex/AGENTS.md, …
    skills/review/…         -> ~/.claude/skills/review/…, ~/.codex/skills/review/…, …
  dot.toml                  optional: values, machines, agents, sync, explicit mappings
```

- `home/X` belongs at `~/X`. Nothing else is needed for an ordinary dotfile.
- `agents/<kind>` belongs wherever each installed agent keeps that kind.
- A more specific place wins: if `home/.claude/skills/review` exists, Claude Code gets that one
  and the other agents get `agents/skills/review`.
- A file whose name ends in `.tmpl` is a template. `home/.gitconfig.tmpl` renders to
  `~/.gitconfig` with `{{name}}` values from `dot.toml`, as templates do today.

## Agents

dot ships a small table of agents and where each keeps things:

| Agent | Installed when this exists | instructions | skills | commands |
| --- | --- | --- | --- | --- |
| claude | `~/.claude` | `CLAUDE.md` | `skills/` | `commands/` |
| codex | `~/.codex` | `AGENTS.md` | `skills/` | `prompts/` |
| opencode | `~/.config/opencode` | `AGENTS.md` | `skills/` | `commands/` |
| pi | `~/.pi/agent` | `AGENTS.md` | `skills/` | `prompts/` |
| cursor | `~/.cursor` | none | `skills/` | none |

Rules:

- A place is used on a machine only when its agent is installed there. dot never creates an
  agent's home folder.
- The table is data. `dot.toml` can add an agent or change one:

  ```toml
  [agent.myharness]
  home = "~/.myharness"
  instructions = "RULES.md"
  skills = "skills"
  ```

- A kind is only a name. If any agent defines `rules = "rules"`, then `agents/rules/` in the
  setup is shared with every agent that defines `rules`. `instructions`, `skills` and `commands`
  are simply the kinds the built-in table uses.
- To send something to some agents only: `[only] "agents/skills/browser" = ["claude", "codex"]`.
  The same table limits a path to some machines: `"home/.config/hypr" = ["desktop"]`.
- `dot agents` prints the table as it applies on this machine: which agents are installed and
  where each kind goes.

Commands and prompts are in the table, but agents disagree about their format (front matter
differs), so sharing them only works for plain prompts. Format translation is out of scope.

## Two-way sync

dot records, for every file it writes, the hash of what it wrote (`.state/written.json`, as
today) and the commit it applied (`.state/applied`, new). With those it can tell which side
changed since the last sync:

| Live file | Setup | What sync does |
| --- | --- | --- |
| same as written | same | nothing |
| same as written | changed | apply the setup's version |
| changed | same | take the live version into the setup |
| changed | changed, to the same bytes | record it |
| changed | changed, differently | merge the two with the last applied version as the base; if the merge is clean, use it; otherwise leave the live file alone and report a conflict |

`dot sync`, in order:

1. **Take.** Copy every taken file into the setup. For a file with several names, take whichever
   name changed and write it to the other names; if two names changed differently, merge them
   the same way.
2. **Commit** what was taken, in one commit named for the machine and the paths.
3. **Pull** with rebase. dot's commits are small and replay cleanly in the usual case. If the
   rebase conflicts, abort it, report the conflict, and keep going with the local setup.
4. **Push.**
5. **Apply** the setup to the machine, writing every name of every file.

Inside a managed folder (a skill, a config folder), a new live file is taken as an addition and
a deleted live file is taken as a deletion. A managed path that disappears entirely is restored,
because that is far more often an accident. `dot forget <path>` is how a path stops being
managed.

### Templates

A rendered file differs from its template only on lines that hold a `{{name}}`. To take an edit
to a rendered file, dot applies the edit line by line to the template. If the edit touches a line
that holds a placeholder, dot cannot know which text was the value, so it leaves the file alone
and reports it, as today.

### What dot will not take

Before committing a live edit, dot checks the added lines for things that look like credentials:
private key blocks and the well-known token prefixes (`sk-`, `ghp_`, `AKIA`, and so on). A file
that trips the check is not taken; `status` says so, and `dot take --force <path>` overrides it.
This matters more than it used to: agents write tokens into config files.

## Commands

| Command | What it does |
| --- | --- |
| `dot` / `dot status [path]` | What sync would take, apply, or stop on; with a path, the diff |
| `dot add <path>…` | Start managing a path. An agent's instructions or skill is stored under `agents/` and shared; anything else goes under `home/`. `--only` keeps an agent path to its agent |
| `dot forget <path>…` | Stop managing a path. The live file stays |
| `dot sync` | Take, commit, pull, push, apply |
| `dot apply [-n] [--force]` | Only apply the setup to this machine |
| `dot take <path>` | Only take one path into the setup |
| `dot agents` | The agents installed here and where each kind goes |
| `dot timer [--remove]` | Run `dot sync` on a timer |
| `dot init [remote]` | Create the setup, or clone it |

## Moving from dot today

`[files]` and `[templates]` keep working. The folders are a second way to say the same thing, and
both feed one plan. A setup can move one mapping at a time: `home/.tmux.conf` replaces
`"tmux/.tmux.conf" = "~/.tmux.conf"`. A mapping whose destination depends on a value, such as
`"code/README.md" = "{{workspace_root}}/README.md"`, stays an explicit mapping.

`version = 2` in `dot.toml` marks a setup that uses the folders, so an older dot refuses it
instead of applying half of it.

## Questions for review

1. Is taking live edits by default safe enough, given the credential check and that every take is
   a commit you can revert? Or should a machine opt in?
2. Does restoring a deleted managed path but propagating deletions inside a managed folder match
   what people expect, or is it a trap?
3. Rebase on every sync: what goes wrong when two machines edit the same file between syncs, and
   does aborting and reporting leave the setup in a state the next sync recovers from?
4. Is the `.tmpl` suffix the right way to mark a template, against listing templates in `dot.toml`?
5. What does this design make harder than dot today?
