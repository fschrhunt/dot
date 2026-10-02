# Agents

Coding agents keep the same things under different names. dot treats each of them as one file
or folder with several names, so an edit under one name reaches the rest.

## What is shared

An entry in the setup's `agents/` folder is written to every installed agent that has a place
for its kind:

| In the setup | Kind | Written to |
| --- | --- | --- |
| `agents/instructions.md` | instructions | `~/.claude/CLAUDE.md`, `~/.codex/AGENTS.md`, … |
| `agents/skills/review/` | skills | `~/.claude/skills/review/`, `~/.codex/skills/review/`, … |

The kind is the entry's name up to its first dot. A folder's children are shared one by one, so
each skill is its own unit.

## The agents dot knows

```sh
dot agents
# claude     ~/.claude  (installed)
#   instructions   ~/.claude/CLAUDE.md
#   skills         ~/.claude/skills
# codex      ~/.codex  (installed)
#   instructions   ~/.codex/AGENTS.md
#   skills         ~/.codex/skills
# opencode   ~/.config/opencode  (not installed)
# …
```

Built in: `claude`, `codex`, `opencode`, `pi` and `cursor`. An agent counts as installed when
its folder exists. dot writes only to installed agents and never creates an agent's folder, so a
machine without Codex gets no `~/.codex`. When one agent's folder is a link to another's, such
as `~/.codex/skills` pointing at `~/.claude/skills`, dot writes the one real copy. A hidden file
in `agents/`, such as `.DS_Store`, is ignored.

## Add or change an agent

```toml
[agent.myagent]
home = "~/.myagent"
instructions = "RULES.md"
skills = "skills"
```

`home` is the agent's folder and every other key is a kind, with its path inside that folder.
A table for a built-in agent changes only the keys it sets:

```toml
[agent.cursor]
home = ""            # turn an agent off
```

A kind is only a name. If an agent defines `rules = "rules"`, then `agents/rules/` in the setup
is shared with every agent that defines `rules`.

## Limit what an agent gets

```toml
[only]
"agents/skills/browser" = { agents = ["claude", "codex"] }
```

The skill reaches those two agents and no others. To give one agent its own version of a shared
skill, leave it out with `[only]` and add its version with `dot add --only`, which stores the
path under `home/` for that agent alone.

## What does not carry over

dot shares files as they are. Instructions and skills use the same format across agents;
commands and prompts do not, so dot has no built-in kind for them. Keep each agent's commands
under `home/`.
