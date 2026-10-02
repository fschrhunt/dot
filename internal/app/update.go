package app

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/fschrhunt/dot/internal/setup"
	"github.com/fschrhunt/dot/internal/update"
)

// terminal reports whether dot is talking to a person at a terminal. Tests set it to script
// both answers; the updater never nudges a pipe or an agent.
var terminal = update.Terminal

// Update replaces a directly installed dot with the latest release, or says how to update one
// Homebrew or go install manages. Check only reports whether one is out: exit 1 when it is.
func Update(paths setup.Paths, version string, check bool, out io.Writer) (int, error) {
	if !update.Release(version) {
		fmt.Fprintf(out, "dot: this is a development build (%s); update it from source\n", version)
		return 1, nil
	}
	latest, e := update.Latest(15 * time.Second)
	if e != nil {
		return 2, e
	}
	remember(paths, latest)
	if !update.Newer(latest, version) {
		fmt.Fprintf(out, "dot: %s is the latest\n", version)
		return 0, nil
	}
	method := update.Method()
	if check || method != "direct" {
		fmt.Fprintf(out, "dot: %s is out (you have %s); update with: %s\n", latest, version, update.Hint(method))
		if check {
			return 1, nil
		}
		return 0, nil
	}
	fmt.Fprintf(out, "dot: updating %s to %s\n", version, latest)
	if e := update.Apply(latest); e != nil {
		return 2, e
	}
	fmt.Fprintf(out, "dot: updated to %s; see what's new: https://github.com/fschrhunt/dot/releases/tag/%s\n", latest, latest)
	return 0, nil
}

// checked is the once-a-day record of the latest release, beside sync's bookkeeping.
type checked struct {
	At     time.Time `json:"at"`
	Latest string    `json:"latest"`
}

// remember records the latest release, so the notice needn't ask again today.
func remember(paths setup.Paths, latest string) {
	b, _ := json.Marshal(checked{time.Now(), latest})
	os.MkdirAll(paths.State, 0700)
	os.WriteFile(filepath.Join(paths.State, "update-check"), b, 0600)
}

// Notice tells a person at a terminal, at most once a day, that a newer dot is out. It never
// speaks in a pipe, to an agent, or for a development build, and DOT_NO_UPDATE_CHECK silences it.
func Notice(paths setup.Paths, version string, stderr io.Writer) {
	if !update.Release(version) || os.Getenv("DOT_NO_UPDATE_CHECK") != "" || !terminal(os.Stderr) {
		return
	}
	var c checked
	if b, e := os.ReadFile(filepath.Join(paths.State, "update-check")); e == nil {
		json.Unmarshal(b, &c)
	}
	if time.Since(c.At) > 24*time.Hour {
		latest, e := update.Latest(1500 * time.Millisecond)
		if e != nil {
			return
		}
		remember(paths, latest)
		c.Latest = latest
		if update.Newer(latest, version) {
			fmt.Fprintf(stderr, "\ndot %s is out (you have %s): %s\n", latest, version, update.Hint(update.Method()))
		}
	}
}
