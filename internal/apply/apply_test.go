package apply_test

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"

	"github.com/fschrhunt/dot/internal/testutil"
)

// mode reads permissions of a live path for copying-policy assertions.
func mode(t *testing.T, p string) os.FileMode {
	t.Helper()
	i, e := os.Stat(p)
	if e != nil {
		t.Fatal(e)
	}
	return i.Mode().Perm()
}

// TestTemplateRendersMachineOverrides pins the behavior: template renders machine overrides.
func TestTemplateRendersMachineOverrides(t *testing.T) {
	f := testutil.New(t)
	f.Config("[values]\nroot = \"~/Code\"\n[machine.server]\nroot = \"~/src\"\n[templates]\n\"t.md\" = \"~/t.md\"\n", map[string]string{"t.md": "{{machine}} {{root}}\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "t.md"), "laptop ~/Code\n")
	f.Paths.Machine = "server"
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "t.md"), "server ~/src\n")
}

// TestSourceFansOutToEveryDestination pins the behavior: source fans out to every destination.
func TestSourceFansOutToEveryDestination(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\n\"a.txt\" = [\"~/x/a.txt\", \"~/y/a.txt\"]\n", map[string]string{"a.txt": "a\n"})
	testutil.OK(t, f.Apply(false))
	for _, p := range []string{"x/a.txt", "y/a.txt"} {
		testutil.Equal(t, f.Read(f.Paths.Home, p), "a\n")
	}
}

// TestWriteKeepsExistingModeAndNewFileTakesSourceMode pins the behavior: write keeps existing mode and new file takes source mode.
func TestWriteKeepsExistingModeAndNewFileTakesSourceMode(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nrun = \"~/bin/run\"\n", map[string]string{"run": "1"})
	src, live := filepath.Join(f.Paths.Dot, "run"), filepath.Join(f.Paths.Home, "bin/run")
	if e := os.Chmod(src, 0755); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, mode(t, live), os.FileMode(0755))
	if e := os.Chmod(live, 0700); e != nil {
		t.Fatal(e)
	}
	f.Write(f.Paths.Dot, map[string]string{"run": "2"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "bin/run"), "2")
	testutil.Equal(t, mode(t, live), os.FileMode(0700))
}

// TestSymlinkInSourceIsCopiedAsSymlink pins the behavior: symlink in source is copied as symlink.
func TestSymlinkInSourceIsCopiedAsSymlink(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/real": "x"})
	if e := os.Symlink("real", filepath.Join(f.Paths.Dot, "d/link")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	link, e := os.Readlink(filepath.Join(f.Paths.Home, "d/link"))
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, link, "real")
}

// TestMachinesLimitsMapping pins the behavior: machines limits mapping.
func TestMachinesLimitsMapping(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = { to = \"~/a\", machines = [\"server\"] }\n", map[string]string{"a": "1"})
	testutil.OK(t, f.Apply(false))
	if _, e := os.Stat(filepath.Join(f.Paths.Home, "a")); !os.IsNotExist(e) {
		t.Fatalf("unexpected live path: %v", e)
	}
	f.Paths.Machine = "server"
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "1")
}

// TestMachineNameInSourcePath pins the behavior: machine name in source path.
func TestMachineNameInSourcePath(t *testing.T) {
	f := testutil.New(t)
	f.Config("[machine.server]\n[files]\n\"cfg.{{machine}}.json\" = \"~/cfg.json\"\n", map[string]string{"cfg.laptop.json": "laptop", "cfg.server.json": "server"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "cfg.json"), "laptop")
}

// TestNewFolderTakesSourceMode pins the behavior: new folder takes source mode.
func TestNewFolderTakesSourceMode(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/private/f": "1"})
	if e := os.Chmod(filepath.Join(f.Paths.Dot, "d/private"), 0700); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, mode(t, filepath.Join(f.Paths.Home, "d/private")), os.FileMode(0700))
}

// TestFileBecomingFolderConverges pins the behavior: file becoming folder converges.
func TestFileBecomingFolderConverges(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/x": "1"})
	testutil.OK(t, f.Apply(false))
	if e := os.Remove(filepath.Join(f.Paths.Dot, "d/x")); e != nil {
		t.Fatal(e)
	}
	f.Write(f.Paths.Dot, map[string]string{"d/x/f": "2"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "d/x/f"), "2")
	testutil.Equal(t, f.Status(""), testutil.Result{Code: 0, Output: "up to date\n"})
}

// TestFolderBecomingFileConverges pins the behavior: folder becoming file converges.
func TestFolderBecomingFileConverges(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/x/f": "1"})
	if e := os.Mkdir(filepath.Join(f.Paths.Dot, "d/empty"), 0755); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	for _, p := range []string{"d/x", "d/empty"} {
		if e := os.RemoveAll(filepath.Join(f.Paths.Dot, p)); e != nil {
			t.Fatal(e)
		}
	}
	f.Write(f.Paths.Dot, map[string]string{"d/x": "2", "d/empty": "3"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "d/x"), "2")
	testutil.Equal(t, f.Read(f.Paths.Home, "d/empty"), "3")
	testutil.Equal(t, f.Status(""), testutil.Result{Code: 0, Output: "up to date\n"})
}

// TestFolderReplacedBySymlinkIsNotWrittenThrough pins the behavior: folder replaced by symlink is not written through.
func TestFolderReplacedBySymlinkIsNotWrittenThrough(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1", "d/g": "1"})
	testutil.OK(t, f.Apply(false))
	outside := filepath.Join(f.Temp, "outside")
	if e := os.Rename(filepath.Join(f.Paths.Home, "d"), outside); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(f.Paths.Home, "d")); e != nil {
		t.Fatal(e)
	}
	f.Write(f.Paths.Dot, map[string]string{"d/f": "2"})
	if e := os.Remove(filepath.Join(f.Paths.Dot, "d/g")); e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, f.Apply(false).Code, 1)
	testutil.Equal(t, f.Read(outside, "f"), "1")
	entries, e := os.ReadDir(outside)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, len(entries), 2)
}

// TestEditedFileSkippedBySyncAndRefusedUntilForce pins the behavior: edited file skipped by sync and refused until force.
func TestEditedFileSkippedBySyncAndRefusedUntilForce(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	f.Git(f.Paths.Dot, "init", "-q")
	f.Commit(f.Paths.Dot, map[string]string{".gitignore": ".state/\n"}, "setup")
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"a": "mine"})
	f.Commit(f.Paths.Dot, map[string]string{"a": "2"}, "change")
	testutil.Equal(t, f.Sync().Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "mine")
	if !strings.Contains(f.Read(f.Paths.State, "sync.log"), "edited here: ~/a (dot take ~/a, or dot apply --force)") {
		t.Fatal("missing edited note")
	}
	testutil.Equal(t, f.Apply(false).Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "mine")
	testutil.OK(t, f.Apply(true))
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "2")
}

// TestMirrorDeletesExtraFilesAndEmptyFolders pins the behavior: mirror deletes extra files and empty folders.
func TestMirrorDeletesExtraFilesAndEmptyFolders(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"d/extra": "x", "d/sub/extra": "x", "d/keep.tmp": "x"})
	f.Config("exclude = [\"*.tmp\"]\n[files]\nd = \"~/d\"\n", nil)
	testutil.OK(t, f.Apply(false))
	entries, e := os.ReadDir(filepath.Join(f.Paths.Home, "d"))
	if e != nil {
		t.Fatal(e)
	}
	var names []string
	for _, i := range entries {
		names = append(names, i.Name())
	}
	testutil.Equal(t, strings.Join(names, ","), "f,keep.tmp")
}

// TestMirrorOffKeepsAndReportsExtraFiles pins the behavior: mirror off keeps and reports extra files.
func TestMirrorOffKeepsAndReportsExtraFiles(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = { to = \"~/d\", mirror = false }\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"d/extra": "x"})
	testutil.Equal(t, f.Status(""), testutil.Result{Code: 0, Output: "? extra       ~/d/extra\n"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "d/extra"), "x")
}

// TestDroppedFileIsDeleted pins the behavior: dropped file is deleted.
func TestDroppedFileIsDeleted(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/a\"\nd = \"~/x/d\"\n", map[string]string{"a": "1", "d/f": "1"})
	testutil.OK(t, f.Apply(false))
	f.Config("[files]\na = \"~/a\"\n", nil)
	testutil.OK(t, f.Apply(false))
	if _, e := os.Stat(filepath.Join(f.Paths.Home, "x/d")); !os.IsNotExist(e) {
		t.Fatalf("dropped folder remains: %v", e)
	}
	entries, e := os.ReadDir(filepath.Join(f.Paths.Home, "x"))
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, len(entries), 0)
}

// TestDroppedFileEditedHereIsKeptAndReported pins the behavior: dropped file edited here is kept and reported.
func TestDroppedFileEditedHereIsKeptAndReported(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/a\"\nb = \"~/b\"\n", map[string]string{"a": "1", "b": "1"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"b": "mine"})
	f.Config("[files]\na = \"~/a\"\n", nil)
	testutil.Equal(t, f.Apply(false).Code, 1)
	testutil.Equal(t, f.Read(f.Paths.Home, "b"), "mine")
	if !strings.Contains(f.Status("").Output, "! edited here ~/b") {
		t.Fatal("missing edited path")
	}
}

// TestNewlyExcludedFileIsKept pins the behavior: newly excluded file is kept.
func TestNewlyExcludedFileIsKept(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1", "d/g": "1"})
	testutil.OK(t, f.Apply(false))
	f.Config("[files]\nd = { to = \"~/d\", exclude = [\"g\"] }\n", nil)
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "d/g"), "1")
}

// TestPruneKeepsFolderStillInSource pins the behavior: prune keeps folder still in source.
func TestPruneKeepsFolderStillInSource(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/sub/f": "1"})
	testutil.OK(t, f.Apply(false))
	if e := os.Remove(filepath.Join(f.Paths.Dot, "d/sub/f")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	i, e := os.Stat(filepath.Join(f.Paths.Home, "d/sub"))
	if e != nil || !i.IsDir() {
		t.Fatalf("source directory pruned: %v", e)
	}
	testutil.Equal(t, f.Status(""), testutil.Result{Code: 0, Output: "up to date\n"})
}

// TestWrittenStateUsesPythonJSON protects reuse of the record across old and new binaries.
func TestWrittenStateUsesPythonJSON(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/café<&>😀\"\n", map[string]string{"a": "1"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.State, "written.json"), "{\n \""+f.Paths.Home+"/caf\\u00e9<&>\\ud83d\\ude00\": \"6b86b273ff34fce19d6b804eff5a3f5747ada4eaa22f1d49c01e52ddb7875b4b\"\n}")
}

// TestExcludeMatchingDotSkipsSourceContents preserves Python's implicit root segment quirk.
func TestExcludeMatchingDotSkipsSourceContents(t *testing.T) {
	f := testutil.New(t)
	f.Config("exclude = [\".*\"]\n[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1", "d/sub/g": "2"})
	testutil.OK(t, f.Apply(false))
	entries, e := os.ReadDir(filepath.Join(f.Paths.Home, "d"))
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, len(entries), 0)
}

// TestTemplateSymlinkStaysSymlink preserves Python's link-copy behavior after template validation.
func TestTemplateSymlinkStaysSymlink(t *testing.T) {
	f := testutil.New(t)
	f.Config("[templates]\nt = \"~/t\"\n", map[string]string{"target": "{{machine}}\n"})
	if e := os.Symlink("target", filepath.Join(f.Paths.Dot, "t")); e != nil {
		t.Fatal(e)
	}
	testutil.OK(t, f.Apply(false))
	target, e := os.Readlink(filepath.Join(f.Paths.Home, "t"))
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, target, "target")
}

// TestNewUnmappedParentsRespectUmask protects parent creation outside mapped source directories.
func TestNewUnmappedParentsRespectUmask(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/new/a\"\n", map[string]string{"a": "1"})
	old := syscall.Umask(0)
	defer syscall.Umask(old)
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, mode(t, filepath.Join(f.Paths.Home, "new")), os.FileMode(0777))
}

// TestFolderRootBecomingFileLeavesNestedFolders preserves Python's failure at a mapping root.
func TestFolderRootBecomingFileLeavesNestedFolders(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/sub/f": "1"})
	testutil.OK(t, f.Apply(false))
	if e := os.RemoveAll(filepath.Join(f.Paths.Dot, "d")); e != nil {
		t.Fatal(e)
	}
	f.Write(f.Paths.Dot, map[string]string{"d": "2"})
	r := f.Apply(false)
	testutil.Equal(t, r, testutil.Result{Code: 2, Output: "dot: cannot write ~/d: Directory not empty\n"})
	if i, e := os.Stat(filepath.Join(f.Paths.Home, "d/sub")); e != nil || !i.IsDir() {
		t.Fatalf("nested folder changed: %v", e)
	}
}

// TestRunFollowsAChangeOnly pins the behavior: a mapping's command runs after its files change, and not otherwise.
func TestRunFollowsAChangeOnly(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = { to = \"~/a\", run = \"echo $DOT_MACHINE >> ran\" }\nb = \"~/b\"\n", map[string]string{"a": "1", "b": "1"})
	r := f.Status("")
	if !strings.Contains(r.Output, "> run         echo $DOT_MACHINE >> ran\n") {
		t.Fatal(r.Output)
	}
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Dot, map[string]string{"b": "2"})
	testutil.OK(t, f.Apply(false))
	testutil.Equal(t, f.Read(f.Paths.Home, "ran"), "laptop\n")
}

// TestFailedRunIsReported pins the behavior: a failed command is reported and exits 1, after the files are written.
func TestFailedRunIsReported(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = { to = \"~/a\", run = \"exit 3\" }\n", map[string]string{"a": "1"})
	r := f.Apply(false)
	testutil.Equal(t, r.Code, 1)
	if !strings.Contains(r.Output, "dot: run failed: exit 3 (exit status 3)") {
		t.Fatal(r.Output)
	}
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "1")
}
