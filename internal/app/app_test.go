package app_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/app"
	"github.com/fschrhunt/dot/internal/testutil"
)

// TestStatusReportsDifferingBinaryFiles pins the behavior: status reports differing binary files.
func TestStatusReportsDifferingBinaryFiles(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nb = \"~/b\"\n", map[string]string{"b": "\xff\x00"})
	f.Write(f.Paths.Home, map[string]string{"b": "\xfe"})
	testutil.Equal(t, f.Status("~/b"), testutil.Result{Code: 1, Output: "binary files live ~/b and dot ~/b differ\n"})
}

// TestTakeCopiesLiveEditBackToSource pins the behavior: take copies live edit back to source.
func TestTakeCopiesLiveEditBackToSource(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = [\"~/d\",\"~/e\"]\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"d/f": "2", "d/g": "new"})
	testutil.OK(t, f.Take("~/d"))
	testutil.Equal(t, f.Read(f.Paths.Dot, "d/f"), "2")
	testutil.Equal(t, f.Read(f.Paths.Dot, "d/g"), "new")
}

// TestTakeRefusesTemplateDestination pins the behavior: take refuses template destination.
func TestTakeRefusesTemplateDestination(t *testing.T) {
	f := testutil.New(t)
	f.Config("[templates]\nt = \"~/t\"\n", map[string]string{"t": "{{machine}}\n"})
	testutil.OK(t, f.Apply(false))
	f.Write(f.Paths.Home, map[string]string{"t": "edited\n"})
	r := f.Take("~/t")
	testutil.Equal(t, r.Code, 1)
	if !strings.Contains(r.Output, "+edited") {
		t.Fatal(r.Output)
	}
	testutil.Equal(t, f.Read(f.Paths.Dot, "t"), "{{machine}}\n")
}

// TestTakeOfMissingFolderIsError pins the behavior: take of missing folder is error.
func TestTakeOfMissingFolderIsError(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\nd = \"~/d\"\n", map[string]string{"d/f": "1"})
	testutil.OK(t, f.Apply(false))
	if e := os.RemoveAll(filepath.Join(f.Paths.Home, "d")); e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, f.Take("~/d"), testutil.Result{Code: 2, Output: "dot: ~/d does not exist\n"})
}

// TestInitUsesEmbeddedExample pins the behavior: init uses embedded example.
func TestInitUsesEmbeddedExample(t *testing.T) {
	f := testutil.New(t)
	if e := os.Remove(f.Paths.Dot); e != nil {
		t.Fatal(e)
	}
	var out bytes.Buffer
	code, e := app.Init(f.Paths, "", &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 0)
	testutil.Equal(t, f.Read(f.Paths.Dot, ".gitignore"), ".state/\n")
	testutil.OK(t, f.Apply(false))
	if !strings.Contains(f.Read(f.Paths.Home, ".claude/CLAUDE.md"), "on laptop") {
		t.Fatal("example not rendered")
	}
}
