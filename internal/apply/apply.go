// Package apply carries out plans with edited-in-place protection and compatible state files.
package apply

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"syscall"
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

// Run executes the plan, skips paths related to refused edits, and always saves state.
// force permits replacing edited paths; it never authorizes following destination symlinks.
func Run(c *config.Config, force bool) (done, refused []plan.Action, err error) {
	written, e := ReadWritten(c.Paths)
	if e != nil {
		return nil, nil, e
	}
	p, e := plan.Build(c, written)
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
		if a.Mark == "!" && !force {
			refused = append(refused, a)
			continue
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
	return done, refused, nil
}
