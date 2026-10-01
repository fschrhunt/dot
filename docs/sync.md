# Sync and timers

## What sync does

1. Takes `.state/lock`. If another sync holds it, exits quietly with 0.
2. Refuses tracked uncommitted changes in the setup. Untracked files do not block it.
3. Pulls the upstream with `git pull --ff-only --quiet`.
4. Pushes local commits when `[sync] push = true` and the branch is ahead.
5. Applies the local setup, preserving live edits.
6. Appends one line to `.state/sync.log` and replaces `.state/last`.

For compatibility, the setup must have a `.git` directory. A git worktree with a `.git`
file is refused. Use a regular clone for your setup.

Git commands cannot prompt and have a 60-second timeout. SSH uses batch mode and a
five-second connection timeout. Make sure git authentication works without a prompt.

With no upstream, sync logs `no upstream` and applies locally. It never merges, rebases,
resets or commits. A failed pull or push is logged and sync still applies the local setup, so an
offline machine keeps working, but sync exits 1 so the failure shows. Check the log.

```sh
cat ~/.dot/.state/last
# 2026-10-01 12:15:00 laptop pulled; pushed; 2 changed
```

The state folder also holds `written.json`, hashes of the files dot last wrote and `dir`
for managed folder roots. Existing state carries over from the Python version.
Do not commit `.state/`. Do not delete written.json to resolve a live edit.

## Set an upstream

```sh
cd ~/.dot
git remote add origin server:dot.git
git push -u origin HEAD
dot sync
```

Any private git remote works. To create a bare one:

```sh
ssh server git init --bare dot.git
```

## Uncommitted changes

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

Templates must be edited in the setup. take cannot reconstruct a template from its output.
If a dropped file was edited, remove it yourself or use `dot apply --force`.

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
dot install
systemctl --user status dot.timer
journalctl --user -u dot.service
dot install --remove
```

Units live in `~/.config/systemd/user/`. The first run is scheduled two minutes after boot.
Later runs use a 15-minute interval, or the one in [`[sync] every`](config.md#syncevery).
An interval under 15 minutes also sets `AccuracySec`, so systemd does not run it up to a
minute late. The user service manager must be running.

## macOS agent

```sh
dot install
launchctl print "gui/$(id -u)/dot"
cat ~/.dot/.state/launchd.log
dot install --remove
```

The agent lives in `~/Library/LaunchAgents/dot.plist`. It runs at load and every 900 seconds,
or the interval in [`[sync] every`](config.md#syncevery), while the user session is active.

Both timers keep the installing shell's PATH and any explicit DOT_HOME and DOT_MACHINE.
They execute the Go binary's absolute path. Reinstall after moving or replacing an old
Python installation. No Python is needed for the timer.
