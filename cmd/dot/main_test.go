package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/fschrhunt/dot/internal/testutil"
)

// TestApplyNeedsNoPythonOnPath pins the behavior: apply needs no python on path.
func TestApplyNeedsNoPythonOnPath(t *testing.T) {
	f := testutil.New(t)
	f.Config("[files]\na = \"~/a\"\n", map[string]string{"a": "1"})
	t.Setenv("PATH", t.TempDir())
	var out bytes.Buffer
	code, e := dispatch([]string{"apply"}, &out, &out)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 0)
	testutil.Equal(t, f.Read(f.Paths.Home, "a"), "1")
}

// TestVersionWithoutSetup pins the behavior: version without setup.
func TestVersionWithoutSetup(t *testing.T) {
	f := testutil.New(t)
	if e := os.Remove(f.Paths.Dot); e != nil {
		t.Fatal(e)
	}
	for _, command := range []string{"version", "--version"} {
		var out bytes.Buffer
		code, e := dispatch([]string{command}, &out, &out)
		if e != nil {
			t.Fatal(e)
		}
		testutil.Equal(t, code, 0)
		testutil.Equal(t, out.String(), "dev\n")
	}
}

// TestTimerAndItsEarlierName pins the behavior: dot timer installs the timer, and dot install still does.
func TestTimerAndItsEarlierName(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	bin := filepath.Join(f.Temp, "bin")
	f.Write(bin, map[string]string{"systemctl": "#!/bin/sh\nexit 0\n", "launchctl": "#!/bin/sh\nexit 0\n"})
	for _, name := range []string{"systemctl", "launchctl"} {
		if e := os.Chmod(filepath.Join(bin, name), 0755); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	for _, command := range []string{"timer", "install"} {
		var out bytes.Buffer
		code, e := dispatch([]string{command}, &out, &out)
		if e != nil {
			t.Fatal(e)
		}
		testutil.Equal(t, code, 0)
		testutil.Equal(t, out.String(), "Installed: dot sync runs every 15 minutes on laptop.\n")
	}
}

// TestAMistypedFlagDoesNotRunTheCommand pins the behavior: timer and sync refuse an option they do not take, so a mistyped --remove never installs the timer.
func TestAMistypedFlagDoesNotRunTheCommand(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	for _, args := range [][]string{{"timer", "--remvoe"}, {"sync", "--settle"}} {
		var out bytes.Buffer
		code, e := dispatch(args, &out, &out)
		if code != 2 || e == nil || !strings.Contains(e.Error(), "unknown option "+args[1]) {
			t.Fatal(args, code, e)
		}
	}
}
