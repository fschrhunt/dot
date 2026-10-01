// Package schedule installs a systemd user timer or launchd agent running this Go binary.
package schedule

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/fschrhunt/dot/internal/setup"
)

// quote escapes one systemd value, additionally escaping dollar expansion for ExecStart.
func quote(s string, dollar bool) string {
	s = strings.NewReplacer("\\", "\\\\", "\"", "\\\"", "\n", "\\n", "%", "%%").Replace(s)
	if dollar {
		s = strings.ReplaceAll(s, "$", "$$")
	}
	return "\"" + s + "\""
}

// run captures service-manager diagnostics, optionally ignoring a failed unload.
func run(check bool, args ...string) error {
	cmd := exec.Command(args[0], args[1:]...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	e := cmd.Run()
	if e != nil && check {
		return setup.Fail(strings.Join(args, " ") + " failed: " + strings.TrimSpace(stderr.String()))
	}
	return nil
}

// escaped quotes text for a launchd plist string or key.
func escaped(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// span words an interval of whole seconds in its largest exact unit: "15 minutes", "1 hour".
func span(d time.Duration) string {
	n, unit := int64(d/time.Second), "second"
	if d%time.Hour == 0 {
		n, unit = int64(d/time.Hour), "hour"
	} else if d%time.Minute == 0 {
		n, unit = int64(d/time.Minute), "minute"
	}
	if n != 1 {
		unit += "s"
	}
	return fmt.Sprintf("%d %s", n, unit)
}

// systemdSpan writes an interval as systemd reads it, in minutes when exact and else seconds.
func systemdSpan(d time.Duration) string {
	if d%time.Minute == 0 {
		return fmt.Sprintf("%dmin", d/time.Minute)
	}
	return fmt.Sprintf("%ds", d/time.Second)
}

// Run installs or removes the timer in paths.Home, retaining PATH and explicit setup overrides.
// The timer runs dot sync once per every, which must be a positive number of whole seconds.
func Run(paths setup.Paths, every time.Duration, remove bool, out io.Writer) (int, error) {
	program, e := os.Executable()
	if e != nil {
		return 2, e
	}
	program = setup.Real(program)
	type pair struct{ k, v string }
	env := []pair{{"PATH", os.Getenv("PATH")}}
	if os.Getenv("DOT_HOME") != "" {
		env = append(env, pair{"DOT_HOME", paths.Dot})
	}
	if os.Getenv("DOT_MACHINE") != "" {
		env = append(env, pair{"DOT_MACHINE", os.Getenv("DOT_MACHINE")})
	}
	if runtime.GOOS == "darwin" {
		plist := filepath.Join(paths.Home, "Library", "LaunchAgents", "dot.plist")
		domain := fmt.Sprintf("gui/%d", os.Getuid())
		_ = run(false, "launchctl", "bootout", domain+"/dot")
		if remove {
			if e := os.Remove(plist); e != nil && !os.IsNotExist(e) {
				return 2, e
			}
		} else {
			if e := os.MkdirAll(filepath.Dir(plist), 0777); e != nil {
				return 2, e
			}
			if e := os.MkdirAll(paths.State, 0777); e != nil {
				return 2, e
			}
			var b strings.Builder
			b.WriteString("<?xml version=\"1.0\" encoding=\"UTF-8\"?>\n<!DOCTYPE plist PUBLIC \"-//Apple//DTD PLIST 1.0//EN\" \"http://www.apple.com/DTDs/PropertyList-1.0.dtd\">\n<plist version=\"1.0\"><dict>\n<key>Label</key><string>dot</string>\n<key>ProgramArguments</key><array><string>" + escaped(program) + "</string><string>sync</string></array>\n<key>RunAtLoad</key><true/>\n<key>StartInterval</key><integer>" + fmt.Sprint(int64(every/time.Second)) + "</integer>\n<key>EnvironmentVariables</key><dict>")
			for _, p := range env {
				b.WriteString("<key>" + escaped(p.k) + "</key><string>" + escaped(p.v) + "</string>")
			}
			log := escaped(filepath.Join(paths.State, "launchd.log"))
			b.WriteString("</dict>\n<key>StandardOutPath</key><string>" + log + "</string>\n<key>StandardErrorPath</key><string>" + log + "</string>\n</dict></plist>\n")
			if e := os.WriteFile(plist, []byte(b.String()), 0666); e != nil {
				return 2, e
			}
			if e := run(true, "launchctl", "bootstrap", domain, plist); e != nil {
				return 2, e
			}
		}
	} else if runtime.GOOS == "linux" {
		units := filepath.Join(paths.Home, ".config", "systemd", "user")
		if remove {
			_ = run(false, "systemctl", "--user", "disable", "--now", "dot.timer")
			for _, name := range []string{"dot.service", "dot.timer"} {
				if e := os.Remove(filepath.Join(units, name)); e != nil && !os.IsNotExist(e) {
					return 2, e
				}
			}
			_ = run(false, "systemctl", "--user", "daemon-reload")
		} else {
			if e := os.MkdirAll(units, 0777); e != nil {
				return 2, e
			}
			environment := ""
			for _, p := range env {
				environment += "Environment=" + quote(p.k+"="+p.v, false) + "\n"
			}
			service := "[Unit]\nDescription=dot sync\n\n[Service]\nType=oneshot\n" + environment + "ExecStart=" + quote(program, true) + " " + quote("sync", true) + "\n"
			// systemd fires a timer up to a minute late by default. That is a fifteenth of the
			// default interval, so a shorter interval keeps the same proportion.
			accuracy := ""
			if every < 15*time.Minute {
				accuracy = "AccuracySec=" + systemdSpan(max(every/15, time.Second).Truncate(time.Second)) + "\n"
			}
			timer := "[Unit]\nDescription=dot sync every " + span(every) + "\n\n[Timer]\nOnBootSec=2min\nOnUnitActiveSec=" + systemdSpan(every) + "\n" + accuracy + "Persistent=true\n\n[Install]\nWantedBy=timers.target\n"
			if e := os.WriteFile(filepath.Join(units, "dot.service"), []byte(service), 0666); e != nil {
				return 2, e
			}
			if e := os.WriteFile(filepath.Join(units, "dot.timer"), []byte(timer), 0666); e != nil {
				return 2, e
			}
			if e := run(true, "systemctl", "--user", "daemon-reload"); e != nil {
				return 2, e
			}
			if e := run(true, "systemctl", "--user", "enable", "--now", "dot.timer"); e != nil {
				return 2, e
			}
		}
	} else {
		return 2, setup.Fail("install supports Linux and macOS, not " + runtime.GOOS)
	}
	if remove {
		fmt.Fprintln(out, "Removed the dot timer.")
	} else {
		fmt.Fprintf(out, "Installed: dot sync runs every %s on %s.\n", span(every), paths.Machine)
	}
	return 0, nil
}
