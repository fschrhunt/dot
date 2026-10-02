package sync_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	dotsync "github.com/fschrhunt/dot/internal/sync"
	"github.com/fschrhunt/dot/internal/testutil"
)

// TestSyncFastForwardsAndApplies pins the behavior: sync fast forwards and applies.
func TestSyncFastForwardsAndApplies(t *testing.T) {
	f := testutil.New(t)
	bare := f.Remote("[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	other := filepath.Join(f.Temp, "other")
	f.Git(f.Temp, "clone", "-q", bare, other)
	f.Commit(other, map[string]string{"a": "2"}, "change")
	f.Git(other, "push", "-q")
	testutil.Equal(t, f.Sync(), testutil.Result{})
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "2")
}

// TestSyncPushesWhenAheadAndPushIsOn pins the behavior: sync pushes when ahead and push is on.
func TestSyncPushesWhenAheadAndPushIsOn(t *testing.T) {
	f := testutil.New(t)
	bare := f.Remote("[sync]\npush = true\n[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	f.Commit(f.Paths.Dot, map[string]string{"a": "2"}, "local")
	testutil.OK(t, f.Sync())
	testutil.Equal(t, strings.TrimSpace(f.Git(bare, "log", "-1", "--format=%s")), "local")
}

// TestSyncDoesNotPushWhenPushIsOff pins the behavior: sync does not push when push is off.
func TestSyncDoesNotPushWhenPushIsOff(t *testing.T) {
	f := testutil.New(t)
	bare := f.Remote("[sync]\npush = false\n[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	f.Commit(f.Paths.Dot, map[string]string{"a": "2"}, "local")
	testutil.OK(t, f.Sync())
	testutil.Equal(t, strings.TrimSpace(f.Git(bare, "log", "-1", "--format=%s")), "setup")
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "2")
}

// TestSyncRefusesUncommittedChanges pins the behavior: sync refuses uncommitted changes.
func TestSyncRefusesUncommittedChanges(t *testing.T) {
	f := testutil.New(t)
	f.Remote("[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	f.Write(f.Paths.Dot, map[string]string{"a": "dirty"})
	testutil.Equal(t, f.Sync().Code, 1)
	if _, e := os.Stat(filepath.Join(f.Paths.Home, "a")); !os.IsNotExist(e) {
		t.Fatalf("sync wrote live file: %v", e)
	}
	if !strings.Contains(f.Read(f.Paths.State, "last"), "refused: uncommitted changes") {
		t.Fatal("missing refusal")
	}
}

// TestSyncAppliesLocalSetupAfterFailedPull pins the behavior: a failed pull still applies the local
// setup, and the sync exits 1 so the failure is not silent.
func TestSyncAppliesLocalSetupAfterFailedPull(t *testing.T) {
	f := testutil.New(t)
	bare := f.Remote("[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	other := filepath.Join(f.Temp, "other")
	f.Git(f.Temp, "clone", "-q", bare, other)
	f.Commit(other, map[string]string{"a": "remote"}, "remote")
	f.Git(other, "push", "-q")
	f.Commit(f.Paths.Dot, map[string]string{"a": "local"}, "local")
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "local")
	if !strings.Contains(f.Read(f.Paths.State, "last"), "pull failed:") {
		t.Fatal("missing pull failure")
	}
}

// TestSyncExitsQuietlyWhileLocked pins the behavior: sync exits quietly while locked.
func TestSyncExitsQuietlyWhileLocked(t *testing.T) {
	f := testutil.New(t)
	if e := os.Mkdir(f.Paths.State, 0755); e != nil {
		t.Fatal(e)
	}
	lock, e := os.Create(filepath.Join(f.Paths.State, "lock"))
	if e != nil {
		t.Fatal(e)
	}
	defer lock.Close()
	if e := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, f.Sync(), testutil.Result{})
	if _, e := os.Stat(filepath.Join(f.Paths.State, "last")); !os.IsNotExist(e) {
		t.Fatalf("locked sync logged: %v", e)
	}
}

// TestSyncRefusesGitWorktreeSetup preserves the .git directory requirement of the Python command.
func TestSyncRefusesGitWorktreeSetup(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	f.Git(f.Paths.Dot, "init", "-q")
	f.Commit(f.Paths.Dot, map[string]string{".gitignore": ".state/\n"}, "setup")
	worktree := filepath.Join(f.Temp, "setup-worktree")
	f.Git(f.Paths.Dot, "worktree", "add", "--detach", worktree)
	f.Paths.Dot = worktree
	f.Paths.State = filepath.Join(worktree, ".state")
	testutil.Equal(t, f.Sync().Code, 2)
	if !strings.Contains(f.Read(f.Paths.State, "last"), worktree+" is not a git repo") {
		t.Fatal("missing worktree refusal")
	}
}

// overSSH points the setup at a remote git reaches through ssh, with the script as the ssh
// command; the script records its arguments and then runs body.
func overSSH(t *testing.T, f *testutil.Fixture, config, body string) string {
	t.Helper()
	f.Remote(config, map[string]string{"a": "1"})
	script, args := filepath.Join(f.Temp, "myssh"), filepath.Join(f.Temp, "args")
	f.Write(f.Temp, map[string]string{"myssh": "#!/bin/sh\necho \"$@\" > " + args + "\n" + body + "\n"})
	if e := os.Chmod(script, 0755); e != nil {
		t.Fatal(e)
	}
	f.Git(f.Paths.Dot, "remote", "set-url", "origin", "host.invalid:dot.git")
	f.Git(f.Paths.Dot, "config", "core.sshCommand", script)
	return args
}

// TestSyncKeepsYourSSHCommand pins the behavior: sync keeps your ssh command and adds batch mode and the connection timeout.
func TestSyncKeepsYourSSHCommand(t *testing.T) {
	f := testutil.New(t)
	args := overSSH(t, f, "[sync]\nconnect_timeout = \"9s\"\n[files]\na = \"~/a\"\n", "exit 1")
	testutil.Equal(t, f.Sync().Code, 1)
	got, e := os.ReadFile(args)
	if e != nil {
		t.Fatal("the configured ssh command was not used: ", e)
	}
	if !strings.HasPrefix(string(got), "-o BatchMode=yes -o ConnectTimeout=9 ") {
		t.Fatalf("got %q", got)
	}
}

// TestSyncStopsGitAtTheTimeout pins the behavior: sync stops git at the timeout and still applies.
func TestSyncStopsGitAtTheTimeout(t *testing.T) {
	f := testutil.New(t)
	overSSH(t, f, "[sync]\ntimeout = \"1s\"\n[files]\na = \"~/a\"\n", "sleep 30")
	testutil.Equal(t, f.Sync().Code, 1)
	if last := f.Read(f.Paths.State, "last"); !strings.Contains(last, "pull failed: timed out after 1 s; 1 changed") {
		t.Fatal(last)
	}
}

// twoWay seeds a version 2 setup with a remote, applies it, and returns the bare remote.
func twoWay(t *testing.T, f *testutil.Fixture, mappings string, files map[string]string) string {
	t.Helper()
	bare := f.Remote("version = 2\n[files]\n"+mappings, files)
	testutil.OK(t, f.Sync())
	return bare
}

// TestSyncTakesALiveEdit pins the behavior: sync takes a live edit into the setup, commits it under the machine's name and pushes it.
func TestSyncTakesALiveEdit(t *testing.T) {
	f := testutil.New(t)
	bare := twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"a": "2\n"})
	if r := f.Status(""); !strings.Contains(r.Output, "< take        ~/a\n") {
		t.Fatal(r.Output)
	}
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Dot, "a"), "2\n")
	testutil.Equal(t, strings.TrimSpace(f.Git(bare, "log", "-1", "--format=%s")), "laptop: ~/a")
	if last := f.Read(f.Paths.State, "last"); !strings.Contains(last, "1 taken; 0 changed") {
		t.Fatal(last)
	}
}

// TestSyncCarriesAnEditToTheOtherNames pins the behavior: an edit under one name of a file reaches its other names.
func TestSyncCarriesAnEditToTheOtherNames(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = [\"~/CLAUDE.md\", \"~/AGENTS.md\"]\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"AGENTS.md": "2\n"})
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Home, "CLAUDE.md"), "2\n")
	testutil.Equal(t, f.Read(f.Paths.Dot, "a"), "2\n")
}

// TestSyncStopsWhenTwoNamesDiffer pins the behavior: two names of one file edited differently are both left alone and reported.
func TestSyncStopsWhenTwoNamesDiffer(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = [\"~/CLAUDE.md\", \"~/AGENTS.md\"]\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"CLAUDE.md": "claude\n", "AGENTS.md": "codex\n"})
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "CLAUDE.md")+f.Read(f.Paths.Home, "AGENTS.md")+f.Read(f.Paths.Dot, "a"), "claude\ncodex\n1\n")
	if last := f.Read(f.Paths.State, "last"); !strings.Contains(last, "not taken: ~/AGENTS.md (edited differently under another of its names") {
		t.Fatal(last)
	}
}

// TestSyncLeavesAFileChangedOnBothSidesAlone pins the behavior: a file edited here whose source also changed is not taken and not overwritten.
func TestSyncLeavesAFileChangedOnBothSidesAlone(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"a": "here\n"})
	f.Write(f.Paths.Dot, map[string]string{"a": "setup\n"})
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Dot, "a"), "here\nsetup\n")
}

// TestSyncTakesANewFileInAMirroredFolder pins the behavior: a file created in one destination of a folder is taken and written to the others.
func TestSyncTakesANewFileInAMirroredFolder(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "skill = [\"~/.claude/skill\", \"~/.codex/skill\"]\n", map[string]string{"skill/SKILL.md": "1\n"})
	f.Write(f.Paths.Home, map[string]string{".codex/skill/notes/more.md": "new\n"})
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Home, ".claude/skill/notes/more.md")+f.Read(f.Paths.Dot, "skill/notes/more.md"), "new\nnew\n")
}

// TestSyncDoesNotTakeACredential pins the behavior: an edit that adds something shaped like a credential stays out of the setup.
func TestSyncDoesNotTakeACredential(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"a": "1\ntoken = ghp_0123456789abcdefghijklmnopqrstuvwxyz\n"})
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Dot, "a"), "1\n")
	if last := f.Read(f.Paths.State, "last"); !strings.Contains(last, "not taken: ~/a (looks like a credential") {
		t.Fatal(last)
	}
}

// TestSyncRestoresADeletedFile pins the behavior: a deleted live file is restored, never taken as a deletion.
func TestSyncRestoresADeletedFile(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	if e := os.Remove(filepath.Join(f.Paths.Home, "a")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Dot, "a"), "1\n1\n")
}

// TestSyncRebasesItsTakeOntoTheRemote pins the behavior: a take here and a commit from another machine both survive, in the setup and on the remote.
func TestSyncRebasesItsTakeOntoTheRemote(t *testing.T) {
	f := testutil.New(t)
	bare := twoWay(t, f, "a = \"~/a\"\nb = \"~/b\"\n", map[string]string{"a": "1\n", "b": "1\n"})
	other := filepath.Join(f.Temp, "other")
	f.Git(f.Temp, "clone", "-q", bare, other)
	f.Commit(other, map[string]string{"b": "2\n"}, "desktop: ~/b")
	f.Git(other, "push", "-q")
	f.Write(f.Paths.Home, map[string]string{"a": "2\n"})
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Home, "b"), "2\n2\n")
	testutil.Equal(t, f.Git(bare, "log", "--format=%s", "-2"), "laptop: ~/a\ndesktop: ~/b\n")
}

// TestSyncRecordsARebaseConflict pins the behavior: when this machine and the remote changed the same lines, sync undoes the rebase, pushes nothing, keeps the live edit and records the conflict.
func TestSyncRecordsARebaseConflict(t *testing.T) {
	f := testutil.New(t)
	bare := twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	other := filepath.Join(f.Temp, "other")
	f.Git(f.Temp, "clone", "-q", bare, other)
	f.Commit(other, map[string]string{"a": "theirs\n"}, "desktop: ~/a")
	f.Git(other, "push", "-q")
	f.Write(f.Paths.Home, map[string]string{"a": "mine\n"})
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "mine\n")
	testutil.Equal(t, strings.TrimSpace(f.Git(bare, "log", "-1", "--format=%s")), "desktop: ~/a")
	if !strings.HasPrefix(f.Read(f.Paths.State, "conflict"), "conflict: this machine and the remote changed the same lines") {
		t.Fatal(f.Read(f.Paths.State, "conflict"))
	}
	testutil.Equal(t, strings.TrimSpace(f.Git(f.Paths.Dot, "status", "--porcelain", "--untracked-files=no")), "")
	if r := f.Status(""); !strings.HasPrefix(r.Output, "conflict: ") {
		t.Fatal(r.Output)
	}
}

// TestSyncCommitsAnEditMadeInTheSetup pins the behavior: a source edited in the setup itself is committed and applied, not refused.
func TestSyncCommitsAnEditMadeInTheSetup(t *testing.T) {
	f := testutil.New(t)
	bare := twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Dot, map[string]string{"a": "2\n"})
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "2\n")
	testutil.Equal(t, strings.TrimSpace(f.Git(bare, "log", "-1", "--format=%s")), "laptop: setup edited")
}

// TestSettledSyncWaitsForAFileToRest pins the behavior: the timer's sync leaves a file edited in the last minute for its next run.
func TestSettledSyncWaitsForAFileToRest(t *testing.T) {
	f := testutil.New(t)
	twoWay(t, f, "a = \"~/a\"\n", map[string]string{"a": "1\n"})
	f.Write(f.Paths.Home, map[string]string{"a": "2\n"})
	code, e := dotsync.Run(f.Paths, true)
	if e != nil || code != 0 {
		t.Fatal(code, e)
	}
	testutil.Equal(t, f.Read(f.Paths.Home, "a")+f.Read(f.Paths.Dot, "a"), "2\n1\n")
}

// rendered seeds a version 2 setup whose one template has two names, applies it, and returns nothing.
func rendered(t *testing.T, f *testutil.Fixture) {
	t.Helper()
	f.Remote("version = 2\n[templates]\n\"t.md\" = [\"~/CLAUDE.md\", \"~/AGENTS.md\"]\n", map[string]string{"t.md": "Machine {{machine}}\nRules.\n"})
	testutil.OK(t, f.Sync())
}

// TestSyncTakesAnEditToARenderedFile pins the behavior: an edit to a rendered file goes back into its template with the placeholders intact.
func TestSyncTakesAnEditToARenderedFile(t *testing.T) {
	f := testutil.New(t)
	rendered(t, f)
	f.Write(f.Paths.Home, map[string]string{"AGENTS.md": "Machine laptop\nRules.\nMore.\n"})
	testutil.OK(t, f.Sync())
	testutil.Equal(t, f.Read(f.Paths.Dot, "t.md"), "Machine {{machine}}\nRules.\nMore.\n")
	testutil.Equal(t, f.Read(f.Paths.Home, "CLAUDE.md"), "Machine laptop\nRules.\nMore.\n")
}

// TestSyncRefusesAnEditToAFilledLine pins the behavior: an edit to a line the template fills in is not taken.
func TestSyncRefusesAnEditToAFilledLine(t *testing.T) {
	f := testutil.New(t)
	rendered(t, f)
	f.Write(f.Paths.Home, map[string]string{"AGENTS.md": "Machine desk\nRules.\n"})
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Dot, "t.md"), "Machine {{machine}}\nRules.\n")
	if last := f.Read(f.Paths.State, "last"); !strings.Contains(last, "not taken: ~/AGENTS.md (it changes a line that ~/.dot/t.md fills in") {
		t.Fatal(last)
	}
}
