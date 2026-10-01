// Package setup provides setup paths, user errors, and filesystem operations shared by commands.
package setup

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"syscall"
)

// Error is a user-facing problem with a command exit code.
type Error struct {
	Message string
	Code    int
}

// Error returns the diagnostic without a CLI prefix.
func (e *Error) Error() string { return e.Message }

// Fail returns a user-facing error; the default exit code is 2.
func Fail(message string, code ...int) error {
	c := 2
	if len(code) > 0 {
		c = code[0]
	}
	return &Error{message, c}
}

// ExitCode returns the user error's exit code, or 2 for an IO error.
func ExitCode(err error) int {
	var e *Error
	if errors.As(err, &e) {
		return e.Code
	}
	return 2
}

// Paths identifies one home, setup, and machine; commands never infer another home afterward.
type Paths struct{ Home, Dot, State, Machine string }

// FromEnv captures HOME, DOT_HOME and DOT_MACHINE for a command invocation.
func FromEnv() Paths {
	home, _ := os.UserHomeDir()
	p := Paths{Home: home, Machine: os.Getenv("DOT_MACHINE")}
	if p.Machine == "" {
		h, _ := os.Hostname()
		p.Machine = strings.Split(h, ".")[0]
	}
	p.Dot = os.Getenv("DOT_HOME")
	if p.Dot == "" {
		p.Dot = "~/.dot"
	}
	p.Dot = p.Abs(p.Dot)
	p.State = filepath.Join(p.Dot, ".state")
	return p
}

// Expand expands ~ and ~user with the same path contract as configuration destinations.
func (p Paths) Expand(s string) string {
	if s == "~" {
		return p.Home
	}
	if strings.HasPrefix(s, "~/") {
		return p.Home + s[1:]
	}
	if strings.HasPrefix(s, "~") {
		name, rest, _ := strings.Cut(s[1:], "/")
		if u, err := user.Lookup(name); err == nil {
			if rest != "" {
				return filepath.Join(u.HomeDir, rest)
			}
			return u.HomeDir
		}
	}
	return s
}

// Abs expands a user path and makes it absolute.
func (p Paths) Abs(s string) string { a, _ := filepath.Abs(p.Expand(s)); return a }

// Show writes the home directory as ~ in messages.
func (p Paths) Show(s string) string {
	if s == p.Home || strings.HasPrefix(s, p.Home+"/") {
		return "~" + strings.TrimPrefix(s, p.Home)
	}
	return s
}

// Under reports whether p lies strictly inside top, including when top is /.
func Under(p, top string) bool {
	rel, err := filepath.Rel(top, p)
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, "../")
}

// Exists reports whether a path exists, including dangling symlinks.
func Exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// IsLink reports whether the path itself is a symlink.
func IsLink(p string) bool { i, e := os.Lstat(p); return e == nil && i.Mode()&os.ModeSymlink != 0 }

// IsDir follows symlinks, like Python's isdir.
func IsDir(p string) bool { i, e := os.Stat(p); return e == nil && i.IsDir() }

// IsFile follows symlinks and recognizes regular files.
func IsFile(p string) bool { i, e := os.Stat(p); return e == nil && i.Mode().IsRegular() }

// Real resolves existing parents even when the leaf does not exist.
func Real(p string) string {
	if !filepath.IsAbs(p) {
		p, _ = filepath.Abs(p)
	}
	cached := map[string]string{}
	active := map[string]bool{}
	var resolve func(string) string
	resolve = func(path string) string {
		current := "/"
		for _, part := range strings.Split(path, "/") {
			if part == "" || part == "." {
				continue
			}
			if part == ".." {
				current = filepath.Dir(current)
				continue
			}
			candidate := filepath.Join(current, part)
			if result, ok := cached[candidate]; ok {
				current = result
				continue
			}
			target, err := os.Readlink(candidate)
			if err != nil || active[candidate] {
				current = candidate
				continue
			}
			active[candidate] = true
			if !filepath.IsAbs(target) {
				target = current + "/" + target
			}
			current = resolve(target)
			delete(active, candidate)
			cached[candidate] = current
		}
		return current
	}
	return resolve(p)
}

// Excluded matches Python fnmatch globs against each path segment, including backslashes literally.
func Excluded(rel string, patterns []string) bool {
	for _, seg := range strings.Split(rel, "/") {
		for _, pat := range patterns {
			if glob(pat, seg) {
				return true
			}
		}
	}
	return false
}

// glob implements fnmatch's shell patterns without treating backslashes as escapes.
func glob(pat, s string) bool {
	var b strings.Builder
	b.WriteString("(?s)^")
	for i := 0; i < len(pat); i++ {
		switch pat[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			j := i + 1
			if j < len(pat) && pat[j] == '!' {
				j++
			}
			if j < len(pat) && pat[j] == ']' {
				j++
			}
			for j < len(pat) && pat[j] != ']' {
				j++
			}
			if j == len(pat) {
				b.WriteString(`\[`)
				continue
			}
			v := pat[i+1 : j]
			v = strings.ReplaceAll(v, `\`, `\\`)
			if strings.HasPrefix(v, "!") {
				v = "^" + v[1:]
			} else if strings.HasPrefix(v, "^") || strings.HasPrefix(v, "[") {
				v = `\` + v
			}
			b.WriteString("[" + v + "]")
			i = j
		default:
			b.WriteString(regexp.QuoteMeta(string(pat[i])))
		}
	}
	b.WriteByte('$')
	r, e := regexp.Compile(b.String())
	return e == nil && r.MatchString(s)
}

// Hash returns the state-compatible SHA256 digest of bytes.
func Hash(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }

// LiveSig hashes a file or link, returns dir for folders, and empty for absent paths.
func LiveSig(p string) (string, error) {
	if IsLink(p) {
		t, e := os.Readlink(p)
		return Hash([]byte("link:" + t)), e
	}
	if IsDir(p) {
		return "dir", nil
	}
	if IsFile(p) {
		b, e := os.ReadFile(p)
		return Hash(b), e
	}
	return "", nil
}

// Want describes the bytes, type, permissions, and source of a desired destination.
type Want struct {
	Kind     string
	Data     []byte
	Mode     os.FileMode
	Sig, Src string
	Template bool
}

// WantOf reads a source without dereferencing symlinks; data optionally replaces file bytes.
func WantOf(src string, data []byte) (Want, error) {
	w := Want{Src: src}
	if IsLink(src) {
		t, e := os.Readlink(src)
		w.Kind = "link"
		w.Data = []byte(t)
		w.Sig = Hash([]byte("link:" + t))
		return w, e
	}
	i, e := os.Stat(src)
	if e != nil {
		return w, e
	}
	w.Mode = i.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	if i.IsDir() {
		w.Kind = "dir"
		w.Sig = "dir"
		return w, nil
	}
	w.Kind = "file"
	if data == nil {
		data, e = os.ReadFile(src)
	}
	w.Data = data
	w.Sig = Hash(data)
	return w, e
}

// Write atomically replaces a file or symlink, preserving an existing regular file's mode.
func Write(p string, w Want) error {
	parent := filepath.Dir(p)
	if e := os.MkdirAll(parent, 0777); e != nil {
		return e
	}
	if w.Kind == "link" {
		tmp := filepath.Join(parent, fmt.Sprintf(".dot-%d.tmp", os.Getpid()))
		if e := os.Symlink(string(w.Data), tmp); e != nil {
			return e
		}
		return os.Rename(tmp, p)
	}
	mode := w.Mode
	if IsFile(p) && !IsLink(p) {
		i, e := os.Stat(p)
		if e != nil {
			return e
		}
		mode = i.Mode() & (os.ModePerm | os.ModeSetuid | os.ModeSetgid | os.ModeSticky)
	}
	f, e := os.CreateTemp(parent, ".dot-")
	if e != nil {
		return e
	}
	defer os.Remove(f.Name())
	_, e = f.Write(w.Data)
	if e == nil {
		e = f.Sync()
	}
	ce := f.Close()
	if e == nil {
		e = ce
	}
	if e != nil {
		return e
	}
	if e = os.Chmod(f.Name(), mode); e != nil {
		return e
	}
	return os.Rename(f.Name(), p)
}

// Reason extracts the OS error text used in a one-line command error.
func Reason(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		s := errno.Error()
		if len(s) > 0 {
			return strings.ToUpper(s[:1]) + s[1:]
		}
	}
	return strings.ReplaceAll(err.Error(), "\n", "; ")
}

// Walk visits directory trees without following links or entering excluded directories.
// Each callback receives a directory and its files and symlinks in filesystem order.
func Walk(root string, patterns []string, visit func(string, []string) error) error {
	var walk func(string) error
	walk = func(dir string) error {
		f, e := os.Open(dir)
		if e != nil {
			return nil
		}
		entries, e := f.ReadDir(-1)
		f.Close()
		if e != nil {
			return nil
		}
		var files, links, dirs []string
		for _, i := range entries {
			q := filepath.Join(dir, i.Name())
			if i.IsDir() {
				dirs = append(dirs, i.Name())
			} else if i.Type()&os.ModeSymlink != 0 && IsDir(q) {
				links = append(links, i.Name())
			} else {
				files = append(files, i.Name())
			}
		}
		if e = visit(dir, append(files, links...)); e != nil {
			return e
		}
		slices.Sort(dirs)
		for _, d := range dirs {
			q := filepath.Join(dir, d)
			rel, _ := filepath.Rel(root, q)
			if !Excluded(rel, patterns) {
				if e = walk(q); e != nil {
					return e
				}
			}
		}
		return nil
	}
	return walk(root)
}
