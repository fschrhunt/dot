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

// sshCommand returns the ssh command git would use in the setup, with dot's two options added:
// batch mode, so a timer never waits at a prompt, and the connection timeout. The command is
// the user's GIT_SSH_COMMAND or core.sshCommand when one is set, and plain ssh otherwise. With
// only GIT_SSH set, the choice of program is left alone and the result is empty.
func sshCommand(p setup.Paths, connect time.Duration) string {
	base := os.Getenv("GIT_SSH_COMMAND")
	if base == "" {
		out, _ := exec.Command("git", "-C", p.Dot, "config", "--get", "core.sshCommand").Output()
		base = strings.TrimSpace(string(out))
	}
	if base == "" {
		if os.Getenv("GIT_SSH") != "" {
			return ""
		}
		base = "ssh"
	}
	return fmt.Sprintf("%s -o BatchMode=yes -o ConnectTimeout=%d", base, connect/time.Second)
}

// session is one sync's way of running git: the setup, the deadline for each command, and the
// environment that keeps git and ssh from prompting.
type session struct {
	p       setup.Paths
	timeout time.Duration
	env     []string
}

// open prepares git for the setup from this machine's sync settings.
func open(p setup.Paths, s config.Sync) session {
	env := append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	if ssh := sshCommand(p, s.ConnectTimeout); ssh != "" {
		env = append(env, "GIT_SSH_COMMAND="+ssh)
	}
	return session{p, s.Timeout, env}
}

// git runs noninteractive git in the setup, stopping it at the deadline.
func (s session) git(args ...string) (result, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", s.p.Dot}, args...)...)
	cmd.Env = s.env
	cmd.WaitDelay = time.Second
	var out, stderr bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &stderr
	e := cmd.Run()
	if ctx.Err() != nil {
		return result{err: fmt.Sprintf("timed out after %d s", s.timeout/time.Second), code: 124}, nil
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
	// The settings come from dot.toml as it is before the pull, which may be about to change it.
	s := open(p, config.SyncSettings(p))
	r, e := s.git("status", "--porcelain", "--untracked-files=no")
	if e != nil {
		return notes, 2, e
	}
	if strings.TrimSpace(r.out) != "" {
		return notes, 1, setup.Fail("refused: uncommitted changes in "+p.Show(p.Dot)+" (commit or discard them)", 1)
	}
	r, e = s.git("rev-parse", "--abbrev-ref", "@{u}")
	if e != nil {
		return notes, 2, e
	}
	upstream := r.code == 0
	if !upstream {
		notes = append(notes, "no upstream")
	} else {
		head, e := s.git("rev-parse", "HEAD")
		if e != nil {
			return notes, 2, e
		}
		r, e = s.git("pull", "--ff-only", "--quiet")
		if e != nil {
			return notes, 2, e
		}
		if r.code != 0 {
			notes = append(notes, "pull failed: "+lastLine(r.err))
			failed = true
		} else {
			now, e := s.git("rev-parse", "HEAD")
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
		r, e = s.git("rev-list", "--count", "@{u}..HEAD")
		if e != nil {
			return notes, 2, e
		}
		n := strings.TrimSpace(r.out)
		if n != "" && n != "0" {
			r, e = s.git("push", "--quiet")
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
	ran := 0
	for _, a := range done {
		if a.Op == "run" {
			ran++
		}
	}
	notes = append(notes, fmt.Sprintf("%d changed", len(done)-ran))
	if ran > 0 {
		notes = append(notes, fmt.Sprintf("%d run", ran))
	}
	for _, a := range refused {
		notes = append(notes, plan.Refusal(p, a))
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
