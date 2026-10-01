// Package sync fast-forwards the setup, optionally pushes local commits, and applies quietly.
package sync

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/fschrhunt/dot/internal/apply"
	"github.com/fschrhunt/dot/internal/config"
	"github.com/fschrhunt/dot/internal/plan"
	"github.com/fschrhunt/dot/internal/setup"
)

// result holds git's captured output and exit status.
type result struct {
	out, err string
	code     int
}

// git runs noninteractive git in the setup with a 60-second deadline.
func git(p setup.Paths, args ...string) (result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", p.Dot}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=5")
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e := cmd.Run()
	if ctx.Err() != nil {
		return result{err: "timed out after 60 s", code: 124}, nil
	}
	if e != nil {
		if _, ok := e.(*exec.ExitError); !ok {
			return result{}, setup.Fail("git not found on PATH")
		}
	}
	return result{out.String(), stderr.String(), cmd.ProcessState.ExitCode()}, nil
}

// lastLine returns the final diagnostic or ? when git supplied none.
func lastLine(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "?"
	}
	lines := strings.Split(s, "\n")
	return lines[len(lines)-1]
}

// run performs sync while holding the lock. A failed pull or push is noted and the local setup is
// still applied, so an offline machine keeps working, but the sync exits 1 so the failure shows.
func run(p setup.Paths) (notes []string, code int, err error) {
	failed := false
	if !setup.IsDir(filepath.Join(p.Dot, ".git")) {
		return notes, 2, setup.Fail(p.Show(p.Dot) + " is not a git repo")
	}
	r, e := git(p, "status", "--porcelain", "--untracked-files=no")
	if e != nil {
		return notes, 2, e
	}
	if strings.TrimSpace(r.out) != "" {
		return notes, 1, setup.Fail("refused: uncommitted changes in "+p.Show(p.Dot)+" (commit or discard them)", 1)
	}
	r, e = git(p, "rev-parse", "--abbrev-ref", "@{u}")
	if e != nil {
		return notes, 2, e
	}
	upstream := r.code == 0
	if !upstream {
		notes = append(notes, "no upstream")
	} else {
		head, e := git(p, "rev-parse", "HEAD")
		if e != nil {
			return notes, 2, e
		}
		r, e = git(p, "pull", "--ff-only", "--quiet")
		if e != nil {
			return notes, 2, e
		}
		if r.code != 0 {
			notes = append(notes, "pull failed: "+lastLine(r.err))
			failed = true
		} else {
			now, e := git(p, "rev-parse", "HEAD")
			if e != nil {
				return notes, 2, e
			}
			note := "up to date"
			if now.out != head.out {
				note = "pulled"
			}
			notes = append(notes, note)
		}
	}
	c, e := config.Load(p)
	if e != nil {
		return notes, setup.ExitCode(e), e
	}
	if upstream && c.Push {
		r, e = git(p, "rev-list", "--count", "@{u}..HEAD")
		if e != nil {
			return notes, 2, e
		}
		n := strings.TrimSpace(r.out)
		if n != "" && n != "0" {
			r, e = git(p, "push", "--quiet")
			if e != nil {
				return notes, 2, e
			}
			note := "pushed"
			if r.code != 0 {
				note = "push failed: " + lastLine(r.err)
				failed = true
			}
			notes = append(notes, note)
		}
	}
	done, refused, e := apply.Run(c, false)
	if e != nil {
		return notes, setup.ExitCode(e), e
	}
	notes = append(notes, fmt.Sprintf("%d changed", len(done)))
	for _, a := range refused {
		notes = append(notes, plan.EditedNote(p, a.Path, a.Op))
	}
	if len(refused) > 0 || failed {
		code = 1
	}
	return notes, code, nil
}

// Run skips an already locked sync and writes one compatible log and last-result line.
func Run(p setup.Paths) (int, error) {
	if e := os.MkdirAll(p.State, 0777); e != nil {
		return 2, e
	}
	lock, e := os.OpenFile(filepath.Join(p.State, "lock"), os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0666)
	if e != nil {
		return 2, e
	}
	defer lock.Close()
	if e = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); e != nil {
		if e == syscall.EWOULDBLOCK {
			return 0, nil
		}
		return 2, e
	}
	notes, code, e := run(p)
	if e != nil {
		notes = append(notes, strings.ReplaceAll(e.Error(), "\n", "; "))
		code = setup.ExitCode(e)
	}
	line := time.Now().Format("2006-01-02 15:04:05") + " " + p.Machine + " " + strings.Join(notes, "; ") + "\n"
	f, e := os.OpenFile(filepath.Join(p.State, "sync.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0666)
	if e != nil {
		return 2, e
	}
	_, e = f.WriteString(line)
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return 2, e
	}
	e = os.WriteFile(filepath.Join(p.State, "last"), []byte(line), 0666)
	return code, e
}
