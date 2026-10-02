package schedule_test

import (
	"bytes"
	"encoding/xml"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/schedule"
	"github.com/fschrhunt/dot/internal/testutil"
)

// install writes the default timer and returns the systemd service or the launchd agent.
func install(t *testing.T, f *testutil.Fixture) string {
	t.Helper()
	installWith(t, f, config.DefaultSync(1))
	if runtime.GOOS == "darwin" {
		return f.Read(f.Paths.Home, "Library/LaunchAgents/com.fschrhunt.dot.plist")
	}
	return f.Read(f.Paths.Home, ".config/systemd/user/dot.service")
}

// installWith stubs service managers so only the temp home receives units or agents.
func installWith(t *testing.T, f *testutil.Fixture, s config.Sync) {
	t.Helper()
	bin := filepath.Join(f.Temp, "bin")
	f.Write(bin, map[string]string{"systemctl": "#!/bin/sh\nexit 0\n", "launchctl": "#!/bin/sh\nexit 0\n"})
	for _, name := range []string{"systemctl", "launchctl"} {
		if e := os.Chmod(filepath.Join(bin, name), 0755); e != nil {
			t.Fatal(e)
		}
	}
	t.Setenv("PATH", bin+":"+os.Getenv("PATH"))
	var out bytes.Buffer
	code, e := schedule.Run(f.Paths, s, false, &out)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 0)
}

// timer returns the part of the installed timer that holds its interval.
func timer(f *testutil.Fixture) string {
	if runtime.GOOS == "darwin" {
		return f.Read(f.Paths.Home, "Library/LaunchAgents/com.fschrhunt.dot.plist")
	}
	return f.Read(f.Paths.Home, ".config/systemd/user/dot.timer")
}

// TestInstallDefaultTimerIsUnchanged pins the behavior: the default timer is the one earlier versions wrote.
func TestInstallDefaultTimerIsUnchanged(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	installWith(t, f, config.DefaultSync(1))
	if runtime.GOOS == "darwin" {
		if !strings.Contains(timer(f), "<key>StartInterval</key><integer>900</integer>") {
			t.Fatal(timer(f))
		}
		return
	}
	testutil.Equal(t, timer(f), "[Unit]\nDescription=dot sync every 15 minutes\n\n[Timer]\nOnBootSec=2min\nOnUnitActiveSec=15min\nPersistent=true\n\n[Install]\nWantedBy=timers.target\n")
}

// TestInstallWritesTheInterval pins the behavior: install writes the interval and the delay after boot, and a short
// interval keeps systemd punctual.
func TestInstallWritesTheInterval(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	s := config.DefaultSync(1)
	s.Every, s.AfterBoot = 90*time.Second, 10*time.Second
	installWith(t, f, s)
	want := "Description=dot sync every 90 seconds\n\n[Timer]\nOnBootSec=10s\nOnUnitActiveSec=90s\nAccuracySec=6s\n"
	if runtime.GOOS == "darwin" {
		want = "<key>StartInterval</key><integer>90</integer>"
	}
	if !strings.Contains(timer(f), want) {
		t.Fatalf("got %q; want %q", timer(f), want)
	}
}

// TestInstallKeepsMachineName pins the behavior: install keeps machine name.
func TestInstallKeepsMachineName(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	timer := install(t, f)
	if runtime.GOOS == "darwin" {
		if !strings.Contains(timer, "<key>DOT_MACHINE</key><string>laptop</string>") {
			t.Fatal(timer)
		}
	} else if !strings.Contains(timer, "Environment=\"DOT_MACHINE=laptop\"\n") {
		t.Fatal(timer)
	}
}

// TestSystemdUnitEscapesValues pins the behavior: systemd unit escapes values.
func TestSystemdUnitEscapesValues(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("systemd units are Linux only")
	}
	f := testutil.New(t)
	f.Paths.Dot = filepath.Join(f.Paths.Home, "a%b\"c\\d")
	f.Paths.State = filepath.Join(f.Paths.Dot, ".state")
	t.Setenv("DOT_HOME", f.Paths.Dot)
	f.Config("", nil)
	want := "Environment=\"DOT_HOME=" + f.Paths.Home + "/a%%b\\\"c\\\\d\"\n"
	if timer := install(t, f); !strings.Contains(timer, want) {
		t.Fatalf("got %q; want %q", timer, want)
	}
}

// TestInstallRunsBinary pins the behavior: install runs binary.
func TestInstallRunsBinary(t *testing.T) {
	f := testutil.New(t)
	f.Config("", nil)
	timer := install(t, f)
	program, e := os.Executable()
	if e != nil {
		t.Fatal(e)
	}
	program, e = filepath.EvalSymlinks(program)
	if e != nil {
		t.Fatal(e)
	}
	if runtime.GOOS == "linux" {
		if !strings.Contains(timer, "ExecStart=\""+program+"\" \"sync\"\n") {
			t.Fatal(timer)
		}
	} else {
		decoder := xml.NewDecoder(strings.NewReader(timer))
		var args []string
		inArgs := false
		for {
			tok, e := decoder.Token()
			if e != nil {
				break
			}
			if s, ok := tok.(xml.StartElement); ok {
				if s.Name.Local == "array" {
					inArgs = true
				}
				if s.Name.Local == "string" && inArgs {
					var v string
					if e := decoder.DecodeElement(&v, &s); e != nil {
						t.Fatal(e)
					}
					args = append(args, v)
				}
			}
			if s, ok := tok.(xml.EndElement); ok && s.Name.Local == "array" {
				inArgs = false
			}
		}
		testutil.Equal(t, strings.Join(args, "|"), program+"|sync")
	}
}
