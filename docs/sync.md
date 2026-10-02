# Sync and timers

## What sync does

`dot sync` keeps a machine and the setup the same, in both directions. For every file, dot
remembers what it last wrote, so it can tell which side changed:

| The live file | The setup | Sync |
| --- | --- | --- |
| as dot wrote it | unchanged | does nothing |
| as dot wrote it | changed | writes the setup's version |
| edited | unchanged | takes the edit into the setup |
| edited | changed | leaves both alone and reports it |
| differs, and dot never wrote it | any | leaves both alone and reports it |

In order, sync:

1. Takes `.state/lock`. If another sync holds it, exits quietly with 0.
2. Takes each edit into the setup. An edit under one name of a file is written to its other
   names. A new file inside a shared folder, such as a skill, is taken as part of it.
3. Commits what changed in the setup, as `<machine>: <paths>`. If a line being committed looks
   like a credential, or the setup no longer loads, sync stops here and says so.
4. Pulls with `git pull --rebase`, so this machine's commits sit on top of the remote's.
5. Pushes, when `[sync] push` is on.
6. Applies the setup to the machine.
7. Appends one line to `.state/sync.log` and replaces `.state/last`.

`dot status` prints the plan for steps 2 and 6 without doing anything. It cannot know what
step 4 will bring.

A version 1 setup, or a machine with `[sync] take = false`, skips steps 2 and 3, pulls with
`--ff-only`, and refuses to run while the setup has uncommitted changes.

## What sync never does

- **Merge.** A file changed on both sides is yours to settle: `dot take <path>` keeps the live
  file and `dot apply --force` keeps the setup's.
- **Take a deletion.** A managed file you delete is written again. `dot forget <path>` is how a
  path stops being managed.
- **Take or commit what looks like a credential.** An edit that adds a private key block or a
  token with a well-known prefix is held back and reported. Sync also refuses to commit such a
  line however it reached the setup: if it belongs there, `dot take <path>` and commit it
  yourself. This is a seat belt, not a scanner: do not rely on it to keep secrets out.
- **Commit a setup that does not load.** If an edit inside `~/.dot` leaves `dot.toml` or a mapping
  broken, sync stops and names what to fix, so a half-finished edit never reaches other machines.
- **Take an edit to a line a template fills in.** An edit to a rendered file goes back into its
  template, but only on lines the template leaves as they are.
- **Replace a file that changed while it ran.** The file is reported and taken on the next run.
- **Take a file still being written.** The timer leaves a file modified in the last minute for
  its next run, and does nothing at all while a file inside `~/.dot` was edited that recently.
  A sync you run yourself goes ahead at once.
- **Remove a file it did not write.** A new file in a shared folder is taken, never deleted;
  `dot apply` alone lists it as `? extra` and leaves it for sync.

## When two machines change the same lines

Git rebases this machine's commits onto the remote's. Edits to different files, or to different
lines of one file, both survive. If they touch the same lines, sync undoes the rebase, pushes
nothing, applies the local setup, and exits 1. `dot status` then begins with:

```text
conflict: this machine and the remote changed the same lines; in ~/.dot run git pull --rebase, fix the files it names, git rebase --continue, then dot sync
```

The message stays until a pull succeeds. While you are resolving it, sync refuses to run and
leaves your rebase alone:

```text
refused: a rebase is in progress in ~/.dot (git rebase --continue once the files are fixed, or git rebase --abort)
```

For compatibility, the setup must have a `.git` directory. A git worktree with a `.git`
file is refused. Use a regular clone for your setup.

Git commands cannot prompt, and each has a 60-second timeout. SSH uses batch mode and a
five-second connection timeout. Change the timeouts with
[`[sync] timeout` and `connect_timeout`](config.md#synctimeout-and-syncconnect_timeout).
Make sure git authentication works without a prompt.

dot keeps your ssh command. It takes `GIT_SSH_COMMAND` or git's `core.sshCommand` when you
have set one, and plain `ssh` otherwise, and adds `-o BatchMode=yes -o ConnectTimeout=5` to it.
With only `GIT_SSH` set, dot leaves ssh to that program and adds nothing.

With no upstream, sync logs `no upstream` and works locally. A failed pull or push is logged and
sync still applies the local setup, so an offline machine keeps working, but sync exits 1 so the
failure shows. Check the log.

```sh
cat ~/.dot/.state/last
# 2026-10-01 12:15:00 laptop pulled; pushed; 1 taken; 2 changed; 1 ran
```

`1 taken` counts the edits taken into the setup, and `1 ran` the mapping
[`run`](config.md#run) commands that ran; each appears only when it is not zero.

The state folder also holds `written.json`, hashes of the files dot last wrote and `dir`
for managed folder roots.
Do not commit `.state/`. Do not delete written.json to resolve a live edit.

## Set an upstream

```sh
cd ~/.dot
git remote add origin git@github.com:you/dotfiles.git
git push -u origin HEAD
dot sync
```

Any private git remote works: a private repository on a git host, or a bare one on a machine
you reach over ssh:

```sh
ssh you@host git init --bare dotfiles.git
git remote add origin you@host:dotfiles.git
```

## Uncommitted changes

A version 2 setup commits them for you: an edit you make inside `~/.dot` is committed and
applied by the next sync. A one-way setup refuses instead:

```text
refused: uncommitted changes in ~/.dot (commit or discard them)
```

Review and commit the setup, then sync:

```sh
cd ~/.dot
git diff
git add dot.toml zshrc
git commit -m 'Update shell setup'
dot sync
```

## Edited live files

```text
edited here: ~/.zshrc (dot take ~/.zshrc, or dot apply --force)
```

Keep the edit:

```sh
dot take ~/.zshrc
cd ~/.dot
git add zshrc
git commit -m 'Keep local shell edit'
dot sync
```

Replace it with the setup version:

```sh
dot status ~/.zshrc
dot apply --force
```

For a rendered file, take carries the edit into its template when it touches only lines the
template leaves as they are; an edit to a line the template fills in belongs in the template.
If a dropped file was edited, remove it yourself or use `dot apply --force`.

## Take a change back

Every take is a commit, so a bad edit, by you or by an agent, is one command to reverse on
every machine:

```sh
dot log ~/.codex/AGENTS.md     # who changed it, and when
dot undo ~/.codex/AGENTS.md    # back to before its last change
dot sync
```

See [commands](commands.md#undo-path).

## Diverged branches or remote failures

Check the final sync line and git state:

```sh
cat ~/.dot/.state/last
git -C ~/.dot status
git -C ~/.dot log --oneline --graph --all -10
```

Resolve divergence manually in `~/.dot` with git. Review any merge or rebase before completing it.
Afterward, commit the resolved setup and run `dot sync` again.
For connection failures, check the remote URL, access and noninteractive authentication.

## Linux timer

```sh
dot timer
systemctl --user status dot.timer
journalctl --user -u dot.service
dot timer --remove
```

Units live in `~/.config/systemd/user/`. The first run is scheduled two minutes after boot,
or after [`[sync] after_boot`](config.md#syncafter_boot).
Later runs use a 15-minute interval, or the one in [`[sync] every`](config.md#syncevery).
An interval under 15 minutes also sets `AccuracySec`, so systemd does not run it up to a
minute late. The user service manager must be running.

## macOS agent

```sh
dot timer
launchctl print "gui/$(id -u)/com.fschrhunt.dot"
cat ~/.dot/.state/launchd.log
dot timer --remove
```

The agent is `com.fschrhunt.dot`, in `~/Library/LaunchAgents/com.fschrhunt.dot.plist`. An agent
from an earlier version, labeled `dot`, is removed when the timer is installed or removed.
It runs at load and every 900 seconds,
or the interval in [`[sync] every`](config.md#syncevery), while the user session is active.

Both timers keep the installing shell's PATH and any explicit DOT_HOME and DOT_MACHINE.
They run the binary at its absolute path, so run `dot timer` again after moving it.
