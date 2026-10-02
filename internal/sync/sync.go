// Package sync brings the setup and the machine in line: it takes and commits live edits when
// the setup syncs both ways, pulls, optionally pushes, and applies quietly.
package sync

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

// settle is how long a file must have been left alone before the timer takes it.
const settle = time.Minute

// tally turns what sync took, did and refused into the log's notes, and reports whether
// anything was refused. took counts the takes made before the pull; any in done came after.
func tally(p setup.Paths, took int, done, refused []plan.Action) (notes []string, stopped bool) {
	ran, late := 0, 0
	for _, a := range done {
		switch a.Op {
		case "run":
			ran++
		case "take":
			late++
		}
	}
	if took+late > 0 {
		notes = append(notes, fmt.Sprintf("%d taken", took+late))
	}
	notes = append(notes, fmt.Sprintf("%d changed", len(done)-ran-late))
	if ran > 0 {
		notes = append(notes, fmt.Sprintf("%d ran", ran))
	}
	for _, a := range refused {
		notes = append(notes, plan.Refusal(p, a))
	}
	return notes, len(refused) > 0
}

// pull brings the remote's commits in and reports whether it could, with its note. Two-way sync
// rebases, so commits dot made here sit on top of the remote's; a rebase that conflicts is undone
// and recorded in .state/conflict until a later pull succeeds. One-way sync only fast-forwards.
func (s session) pull(rebase bool) (note string, ok bool, err error) {
	before, e := s.git("rev-parse", "@{u}")
	if e != nil {
		return "", false, e
	}
	head, e := s.git("rev-parse", "HEAD")
	if e != nil {
		return "", false, e
	}
	how := "--ff-only"
	if rebase {
		how = "--rebase"
	}
	r, e := s.git("pull", how, "--quiet")
	if e != nil {
		return "", false, e
	}
	marker := filepath.Join(s.p.State, "conflict")
	if r.code != 0 {
		if rebase && s.rebasing() {
			if _, e := s.git("rebase", "--abort"); e != nil {
				return "", false, e
			}
			note = "conflict: this machine and the remote changed the same lines; in " + s.p.Show(s.p.Dot) + " run git pull --rebase, fix the files it names, git rebase --continue, then dot sync"
			return note, false, os.WriteFile(marker, []byte(note+"\n"), 0666)
		}
		return "pull failed: " + lastLine(r.err), false, nil
	}
	if e := os.Remove(marker); e != nil && !os.IsNotExist(e) {
		return "", false, e
	}
	after, e := s.git("rev-parse", "@{u}")
	if e != nil {
		return "", false, e
	}
	now, e := s.git("rev-parse", "HEAD")
	if e != nil {
		return "", false, e
	}
	if after.out != before.out || now.out != head.out {
		return "pulled", true, nil
	}
	return "up to date", true, nil
}

// rebasing reports whether git left a rebase unfinished in the setup.
func (s session) rebasing() bool {
	for _, name := range []string{"rebase-merge", "rebase-apply"} {
		if setup.IsDir(filepath.Join(s.p.Dot, ".git", name)) {
			return true
		}
	}
	return false
}

// protectState keeps dot's bookkeeping out of the shared repository. Sync records written.json,
// sync.log and friends under .state/; committed, they differ on every machine and fight each
// other on every rebase. An unignored .state is repaired by adding it to .git/info/exclude
// (the same place dot init writes for cloned setups), so a hand-made repository is covered
// too. A .state that is already tracked cannot be repaired quietly, since untracking changes
// the shared history: sync refuses and names the fix.
func (s session) protectState() error {
	r, e := s.git("ls-files", ".state")
	if e != nil {
		return e
	}
	if strings.TrimSpace(r.out) != "" {
		return setup.Fail("refused: "+s.p.Show(filepath.Join(s.p.Dot, ".state"))+" is tracked by git, and those bookkeeping files fight every other machine: run git -C "+s.p.Show(s.p.Dot)+" rm -r --cached .state, commit, and sync", 1)
	}
	r, e = s.git("check-ignore", ".state/written.json")
	if e != nil {
		return e
	}
	if r.code == 0 {
		return nil
	}
	dir, e := s.git("rev-parse", "--git-dir")
	if e != nil {
		return e
	}
	gitDir := strings.TrimSpace(dir.out)
	if !filepath.IsAbs(gitDir) {
		gitDir = filepath.Join(s.p.Dot, gitDir)
	}
	exclude := filepath.Join(gitDir, "info", "exclude")
	f, e := os.OpenFile(exclude, os.O_APPEND|os.O_WRONLY, 0666)
	if e != nil {
		return setup.Fail("cannot protect dot's state: " + setup.Reason(e))
	}
	if _, e = f.WriteString("\n.state/\n"); e == nil {
		e = f.Close()
	} else {
		f.Close()
	}
	if e != nil {
		return e
	}
	if r, e = s.git("check-ignore", ".state/written.json"); e != nil || r.code != 0 {
		return setup.Fail("refused: git does not ignore "+s.p.Show(filepath.Join(s.p.Dot, ".state"))+" (add \".state/\" to "+s.p.Show(exclude)+" or .gitignore)", 1)
	}
	return nil
}

// push sends local commits when the branch is ahead, and returns its note, if any.
func (s session) push() (note string, ok bool, err error) {
	r, e := s.git("rev-list", "--count", "@{u}..HEAD")
	if e != nil {
		return "", false, e
	}
	if n := strings.TrimSpace(r.out); n == "" || n == "0" {
		return "", true, nil
	}
	r, e = s.git("push", "--quiet")
	if e != nil {
		return "", false, e
	}
	if r.code != 0 {
		return "push failed: " + lastLine(r.err), false, nil
	}
	return "pushed", true, nil
}

// commit records everything changed in the setup as one commit named for the machine and the
// live paths taken, and reports whether there was anything to record. It refuses when a line
// being added looks like a credential, whatever put it in the setup: that commit is the user's
// to make. A machine with no git identity commits as dot.
func (s session) commit(taken []plan.Action) (bool, error) {
	if r, e := s.git("add", "--all"); e != nil || r.code != 0 {
		return false, orFail(e, "git add failed: "+lastLine(r.err))
	}
	r, e := s.git("diff", "--cached", "--unified=0", "--no-color", "--no-ext-diff")
	if e != nil || r.out == "" {
		return false, e
	}
	file := ""
	for _, line := range strings.Split(r.out, "\n") {
		if name, ok := strings.CutPrefix(line, "+++ b/"); ok {
			file = name
		} else if strings.HasPrefix(line, "+") && plan.Secret(nil, []byte(line[1:])) {
			return false, setup.Fail("refused: "+file+" in "+s.p.Show(s.p.Dot)+" looks like it holds a credential; commit it yourself if it belongs in the setup, or remove it", 1)
		}
	}
	what := "setup edited"
	if len(taken) > 0 {
		var shown []string
		for _, a := range taken {
			shown = append(shown, s.p.Show(a.Path))
		}
		if len(shown) > 3 {
			shown = append(shown[:3], fmt.Sprintf("and %d more", len(shown)-3))
		}
		what = strings.Join(shown, ", ")
	}
	args := []string{"commit", "--quiet", "--message", s.p.Machine + ": " + what}
	if r, e := s.git("config", "user.email"); e != nil {
		return false, e
	} else if r.code != 0 {
		args = append([]string{"-c", "user.name=dot", "-c", "user.email=dot@" + s.p.Machine}, args...)
	}
	if r, e = s.git(args...); e != nil || r.code != 0 {
		return false, orFail(e, "git commit failed: "+lastLine(r.err))
	}
	return true, nil
}

// orFail keeps a real error, and otherwise makes a user-facing one from text.
func orFail(e error, text string) error {
	if e != nil {
		return e
	}
	return setup.Fail(text)
}

// run performs sync while holding the lock. A failed pull or push is noted and the local setup is
// still applied, so an offline machine keeps working, but the sync exits 1 so the failure shows.
// A setup that takes (version 2) syncs both ways: live edits are taken and committed, the remote
// is rebased under them, the result is pushed, and then the setup is applied. settled, for the
// timer, holds back live files modified in the last minute and waits while the setup itself was
// edited that recently.
func run(p setup.Paths, settled bool) (notes []string, code int, err error) {
	failed := false
	if !setup.IsDir(filepath.Join(p.Dot, ".git")) {
		return notes, 2, setup.Fail(p.Show(p.Dot) + " is not a git repo")
	}
	// The settings come from dot.toml as it is before the pull, which may be about to change it.
	settings := config.SyncSettings(p)
	s := open(p, settings)
	if e := s.protectState(); e != nil {
		return notes, setup.ExitCode(e), e
	}
	opt := apply.Options{Take: settings.Take}
	if settled {
		opt.Settle = settle
	}
	taken := 0
	if settings.Take {
		// A rebase left open is the user resolving a conflict, and theirs to finish.
		if s.rebasing() {
			return notes, 1, setup.Fail("refused: a rebase is in progress in "+p.Show(p.Dot)+" (git rebase --continue once the files are fixed, or git rebase --abort)", 1)
		}
		r, e := s.git("status", "--porcelain", "-z", "--untracked-files=all", "--no-renames")
		if e != nil {
			return notes, 2, e
		}
		edited := strings.FieldsFunc(r.out, func(c rune) bool { return c == 0 })
		if settled {
			for _, entry := range edited {
				name := entry[min(3, len(entry)):]
				if i, e := os.Lstat(filepath.Join(p.Dot, name)); e == nil && time.Since(i.ModTime()) < settle {
					return []string{"waiting: " + name + " in the setup was edited in the last minute"}, 0, nil
				}
			}
		}
		// A setup that does not load is not committed: the pull may fix one that is clean, and
		// one edited here is still being edited or needs fixing first.
		c, e := config.Load(p)
		if e != nil && len(edited) > 0 {
			return notes, setup.ExitCode(e), e
		}
		var took []plan.Action
		if e == nil {
			o := opt
			o.TakeOnly = true
			if took, _, e = apply.Run(c, o); e != nil {
				return notes, setup.ExitCode(e), e
			}
		}
		taken = len(took)
		if _, e := s.commit(took); e != nil {
			return notes, setup.ExitCode(e), e
		}
	} else {
		r, e := s.git("status", "--porcelain", "--untracked-files=no")
		if e != nil {
			return notes, 2, e
		}
		if strings.TrimSpace(r.out) != "" {
			return notes, 1, setup.Fail("refused: uncommitted changes in "+p.Show(p.Dot)+" (commit or discard them)", 1)
		}
	}
	r, e := s.git("rev-parse", "--abbrev-ref", "@{u}")
	if e != nil {
		return notes, 2, e
	}
	upstream := r.code == 0
	pulled := false
	if !upstream {
		notes = append(notes, "no upstream")
	} else {
		note, ok, e := s.pull(settings.Take)
		if e != nil {
			return notes, 2, e
		}
		notes = append(notes, note)
		pulled = ok
		failed = !ok
	}
	c, e := config.Load(p)
	if e != nil {
		return notes, setup.ExitCode(e), e
	}
	// Nothing is pushed over a pull that failed: the remote has commits this machine lacks.
	if upstream && c.Push && (pulled || !settings.Take) {
		note, ok, e := s.push()
		if e != nil {
			return notes, 2, e
		}
		if note != "" {
			notes = append(notes, note)
		}
		failed = failed || !ok
	}
	opt.Take = c.Take
	done, refused, e := apply.Run(c, opt)
	if e != nil {
		return notes, setup.ExitCode(e), e
	}
	more, stopped := tally(p, taken, done, refused)
	notes = append(notes, more...)
	if stopped || failed {
		code = 1
	}
	return notes, code, nil
}

// Run skips an already locked sync and writes one compatible log and last-result line. settled
// is how the timer runs it: a file modified in the last minute is left for the next run. On a
// failing (or refused) run it also writes the notes to errw, one "dot: " line each: a silent
// non-zero exit is how sync used to lose every explanation it had.
func Run(p setup.Paths, settled bool, errw io.Writer) (int, error) {
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
	notes, code, e := run(p, settled)
	if e != nil {
		notes = append(notes, strings.ReplaceAll(e.Error(), "\n", "; "))
		code = setup.ExitCode(e)
	}
	if code != 0 {
		for _, note := range notes {
			fmt.Fprintln(errw, "dot: "+note)
		}
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
