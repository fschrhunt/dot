package sync_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

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
