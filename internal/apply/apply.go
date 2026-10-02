// Package apply carries out plans with edited-in-place protection and compatible state files.
package apply

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
	"time"
	"unicode/utf16"

	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
)

// ReadWritten loads the shared written.json record; an absent record is empty.
func ReadWritten(paths setup.Paths) (map[string]string, error) {
	b, e := os.ReadFile(filepath.Join(paths.State, "written.json"))
	if os.IsNotExist(e) {
		return map[string]string{}, nil
	}
	if e != nil {
		return nil, e
	}
	var out map[string]string
	e = json.Unmarshal(b, &out)
	return out, e
}

// save records hashes with Python's indentation and no trailing newline, using atomic replacement.
func save(paths setup.Paths, written map[string]string) error {
	var encoded bytes.Buffer
	encoder := json.NewEncoder(&encoded)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", " ")
	if err := encoder.Encode(written); err != nil {
		return err
	}
	var ascii strings.Builder
	for _, r := range strings.TrimSuffix(encoded.String(), "\n") {
		if r < 128 {
			ascii.WriteRune(r)
		} else if r <= 0xffff {
			fmt.Fprintf(&ascii, "\\u%04x", r)
		} else {
			a, b := utf16.EncodeRune(r)
			fmt.Fprintf(&ascii, "\\u%04x\\u%04x", a, b)
		}
	}
	return setup.Write(filepath.Join(paths.State, "written.json"), setup.Want{Kind: "file", Data: []byte(ascii.String()), Mode: 0644})
}

// Disown removes live paths, and every recorded path inside one of them, from the record of
// what dot wrote, along with any folder record above them that no recorded path still sits in,
// so a later apply leaves those paths alone.
func Disown(paths setup.Paths, live []string) error {
	written, e := ReadWritten(paths)
	if e != nil {
		return e
	}
	for q := range written {
		if slices.ContainsFunc(live, func(gone string) bool { return q == gone || setup.Under(q, gone) }) {
			delete(written, q)
		}
	}
	for q, s := range written {
		if s != "dir" {
			continue
		}
		used := false
		for other := range written {
			used = used || setup.Under(other, q)
		}
		if !used {
			delete(written, q)
		}
	}
	return save(paths, written)
}

// prune removes empty parents up to a dropped root, retaining directories still in the source.
func prune(p string, tops, dropped []string, want map[string]setup.Want) {
	top := ""
	for _, t := range tops {
		if setup.Under(p, t) && len(t) > len(top) {
			top = t
		}
	}
	for d := filepath.Dir(p); top != "" && (setup.Under(d, top) || d == top && slices.Contains(dropped, top)) && want[d].Kind != "dir"; d = filepath.Dir(d) {
		if e := syscall.Rmdir(d); e != nil {
			return
		}
	}
}

// command runs a mapping's command through sh in the home folder, with the machine name in
// DOT_MACHINE and the sync timeout as its deadline. It returns why the command failed, or "".
func command(c *config.Config, text string) string {
	ctx, cancel := context.WithTimeout(context.Background(), c.Timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "sh", "-c", text)
	cmd.Dir = c.Paths.Home
	cmd.Env = append(os.Environ(), "DOT_MACHINE="+c.Paths.Machine)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.WaitDelay = time.Second
	e := cmd.Run()
	if ctx.Err() != nil {
		return fmt.Sprintf("timed out after %d s", c.Timeout/time.Second)
	}
	if e != nil {
		return e.Error()
	}
	return ""
}

// Options are apply's switches. Force permits replacing edited paths; it never authorizes
// following destination symlinks. Take and Settle plan two-way, as plan.Options describes.
// TakeOnly performs just the takes and leaves every live path alone, so sync can commit what it
// took before it pulls.
type Options struct {
	Force, Take, TakeOnly bool
	Settle                time.Duration
}

// Run executes the plan, skips paths related to refused edits, and always saves state. A path
// that changed between the plan and its turn is refused, not replaced; so is a take whose live
// file or source changed. Afterward it runs each
// mapping's command whose destinations changed, once and in configuration order: one that ran
// is appended to done, and one that failed to refused.
func Run(c *config.Config, opt Options) (done, refused []plan.Action, err error) {
	written, e := ReadWritten(c.Paths)
	if e != nil {
		return nil, nil, e
	}
	p, e := plan.Build(c, written, plan.Options{Take: opt.Take || opt.TakeOnly, Settle: opt.Settle})
	if e != nil {
		return nil, nil, e
	}
	var tops, dropped []string
	for _, r := range p.Roots {
		tops = append(tops, r.Path)
	}
	for _, q := range plan.Keys(written) {
		if written[q] == "dir" {
			if _, ok := p.Wants[q]; !ok {
				dropped = append(dropped, q)
			}
		}
	}
	defer func() {
		if e := save(c.Paths, written); e != nil {
			err = e
		}
	}()
	for _, a := range p.Actions {
		skip := a.Op == ""
		for _, r := range refused {
			if setup.Under(a.Path, r.Path) || setup.Under(r.Path, a.Path) {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if a.Op == "take" {
			if a.Mark == "!" {
				refused = append(refused, a)
				continue
			}
			live, e := setup.LiveSig(a.Path)
			if e != nil {
				return done, refused, e
			}
			source, e := setup.LiveSig(a.Want.Src)
			if e != nil {
				return done, refused, e
			}
			if live != a.Live || source != a.Base {
				a.Mark, a.Note = "!", "it changed while dot was working; run dot sync again"
				refused = append(refused, a)
			} else if e := setup.Write(a.Want.Src, a.Want); e != nil {
				return done, refused, setup.Fail("cannot take " + c.Paths.Show(a.Path) + ": " + setup.Reason(e))
			} else {
				done = append(done, a)
			}
			continue
		}
		if opt.TakeOnly {
			continue
		}
		if a.Mark == "!" && !opt.Force {
			refused = append(refused, a)
			continue
		}
		// A folder is exempt: dot's own earlier deletions in this run may have emptied and pruned it.
		if a.Looked && a.Op != "mkdir" && a.Live != "dir" {
			now, e := setup.LiveSig(a.Path)
			if e != nil {
				return done, refused, e
			}
			if now != a.Live {
				a.Mark, a.Note = "!", "it changed while dot was working; run dot sync again"
				refused = append(refused, a)
				continue
			}
		}
		e = nil
		if a.Op != "write" && (setup.IsLink(a.Path) || setup.Exists(a.Path) && !setup.IsDir(a.Path)) {
			e = os.Remove(a.Path)
		} else if a.Op == "write" && setup.IsDir(a.Path) && !setup.IsLink(a.Path) {
			e = syscall.Rmdir(a.Path)
		}
		if e == nil {
			switch a.Op {
			case "write":
				e = setup.Write(a.Path, a.Want)
			case "mkdir":
				if !setup.IsDir(a.Path) {
					e = os.MkdirAll(a.Path, 0777)
					if e == nil {
						e = os.Chmod(a.Path, a.Want.Mode|0700)
					}
				}
			case "delete":
				delete(written, a.Path)
				prune(a.Path, append(slices.Clone(tops), dropped...), dropped, p.Wants)
			}
		}
		if e != nil {
			return done, refused, setup.Fail("cannot " + a.Op + " " + c.Paths.Show(a.Path) + ": " + setup.Reason(e))
		}
		done = append(done, a)
	}
	if !opt.TakeOnly {
		for _, d := range dropped {
			_ = syscall.Rmdir(d)
			if !setup.IsDir(d) {
				delete(written, d)
			}
		}
		for q, s := range written {
			if _, ok := p.Wants[q]; !ok && s != "dir" && !setup.Exists(q) {
				delete(written, q)
			}
		}
	}
	for q, w := range p.Wants {
		live, e := setup.LiveSig(q)
		if e != nil {
			return done, refused, e
		}
		if (w.Kind != "dir" || slices.Contains(tops, q)) && live == w.Sig {
			written[q] = w.Sig
		} else if w.Kind == "dir" && !slices.Contains(tops, q) {
			delete(written, q)
		}
	}
	changed := slices.Clone(done)
	for _, h := range p.Hooks {
		if opt.TakeOnly || !plan.Touched(h.Dests, changed) {
			continue
		}
		a := plan.Action{Mark: ">", Path: h.Command, Op: "run"}
		if a.Note = command(c, h.Command); a.Note != "" {
			refused = append(refused, a)
		} else {
			done = append(done, a)
		}
	}
	return done, refused, nil
}
