package plan

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/fschrhunt/dot/internal/setup"
)

// block is a contiguous match between two line sequences.
type block struct{ a, b, n int }

// matches implements difflib's longest-match recursion, including popular-line suppression.
func matches(a, b []string) []block {
	index := map[string][]int{}
	for j, s := range b {
		index[s] = append(index[s], j)
	}
	if len(b) >= 200 {
		for s, js := range index {
			if len(js) > len(b)/100+1 {
				delete(index, s)
			}
		}
	}
	var out []block
	var search func(int, int, int, int)
	search = func(alo, ahi, blo, bhi int) {
		best := block{alo, blo, 0}
		prev := map[int]int{}
		for i := alo; i < ahi; i++ {
			next := map[int]int{}
			for _, j := range index[a[i]] {
				if j < blo {
					continue
				}
				if j >= bhi {
					break
				}
				n := prev[j-1] + 1
				next[j] = n
				if n > best.n {
					best = block{i - n + 1, j - n + 1, n}
				}
			}
			prev = next
		}
		for best.a > alo && best.b > blo && a[best.a-1] == b[best.b-1] {
			best.a--
			best.b--
			best.n++
		}
		for best.a+best.n < ahi && best.b+best.n < bhi && a[best.a+best.n] == b[best.b+best.n] {
			best.n++
		}
		if best.n == 0 {
			return
		}
		if alo < best.a && blo < best.b {
			search(alo, best.a, blo, best.b)
		}
		out = append(out, best)
		if best.a+best.n < ahi && best.b+best.n < bhi {
			search(best.a+best.n, ahi, best.b+best.n, bhi)
		}
	}
	search(0, len(a), 0, len(b))
	var merged []block
	for _, m := range out {
		if len(merged) > 0 {
			p := &merged[len(merged)-1]
			if p.a+p.n == m.a && p.b+p.n == m.b {
				p.n += m.n
				continue
			}
		}
		merged = append(merged, m)
	}
	return append(merged, block{len(a), len(b), 0})
}

// lines splits text with line endings retained, matching Python splitlines(keepends=True).
func lines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); {
		r, n := utf8.DecodeRuneInString(s[i:])
		i += n
		if strings.ContainsRune("\n\r\v\f\u001c\u001d\u001e\u0085\u2028\u2029", r) {
			if r == '\r' && i < len(s) && s[i] == '\n' {
				i++
			}
			out = append(out, s[start:i])
			start = i
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

// Unrender carries an edit made to a rendered file back to its template. render gives what one
// template line renders to, which is several lines when a value holds a newline, and edited is
// the live file. Lines the edit left alone keep the template's text, placeholders included, and
// take the live file's line ending; lines it changed or added take the live text, which is only
// safe where the template's own line was plain. ok is false when the edit touches a line the
// template fills in, or part of the lines one template line renders to.
// The caller must still render the result and compare it with edited before trusting it.
func Unrender(template, edited string, render func(line string) (string, bool)) (string, bool) {
	split := func(s string) []string {
		parts := strings.SplitAfter(s, "\n")
		return parts[:len(parts)-1+min(1, len(parts[len(parts)-1]))]
	}
	text := func(line string) string { return strings.TrimSuffix(line, "\n") }
	// r is the rendered file line by line, owner the template line each came from, and span how
	// many lines each template line gave.
	t := split(template)
	var r []string
	var owner []int
	span := make([]int, len(t))
	for o, line := range t {
		piece, ok := render(line)
		if !ok {
			return "", false
		}
		for _, s := range split(piece) {
			r, owner = append(r, text(s)), append(owner, o)
			span[o]++
		}
	}
	l := split(edited)
	live := make([]string, len(l))
	for j, line := range l {
		live[j] = text(line)
	}
	var out strings.Builder
	// A template line that renders to nothing has no line to match, so it is kept in its place.
	next := 0
	silent := func(upto int) {
		for ; next < upto; next++ {
			if span[next] == 0 {
				out.WriteString(t[next])
			}
		}
	}
	i, j := 0, 0
	for _, m := range matches(r, live) {
		for ; i < m.a; i++ {
			if o := owner[i]; span[o] != 1 || text(t[o]) != r[i] {
				return "", false
			}
		}
		for ; j < m.b; j++ {
			out.WriteString(l[j])
		}
		for n := 0; n < m.n; {
			o := owner[i+n]
			if i+n > 0 && owner[i+n-1] == o || n+span[o] > m.n {
				return "", false
			}
			silent(o)
			last := l[j+n+span[o]-1]
			out.WriteString(text(t[o]) + last[len(text(last)):])
			n += span[o]
		}
		i, j = i+m.n, j+m.n
	}
	silent(len(t))
	return out.String(), true
}

// span formats a unified diff range, including empty and single-line ranges.
func span(start, end int) string {
	n := end - start
	if n == 1 {
		return fmt.Sprint(start + 1)
	}
	if n == 0 {
		return fmt.Sprintf("%d,0", start)
	}
	return fmt.Sprintf("%d,%d", start+1, n)
}

// unified emits difflib-compatible unified hunks with three context lines.
func unified(out io.Writer, a, b []string, from, to string) {
	type op struct {
		tag            string
		ai, az, bi, bz int
	}
	var ops []op
	i, j := 0, 0
	for _, m := range matches(a, b) {
		tag := ""
		if i < m.a && j < m.b {
			tag = "replace"
		} else if i < m.a {
			tag = "delete"
		} else if j < m.b {
			tag = "insert"
		}
		if tag != "" {
			ops = append(ops, op{tag, i, m.a, j, m.b})
		}
		if m.n > 0 {
			ops = append(ops, op{"equal", m.a, m.a + m.n, m.b, m.b + m.n})
		}
		i = m.a + m.n
		j = m.b + m.n
	}
	if len(ops) == 0 {
		return
	}
	if ops[0].tag == "equal" {
		o := &ops[0]
		o.ai = max(o.ai, o.az-3)
		o.bi = max(o.bi, o.bz-3)
	}
	if ops[len(ops)-1].tag == "equal" {
		o := &ops[len(ops)-1]
		o.az = min(o.az, o.ai+3)
		o.bz = min(o.bz, o.bi+3)
	}
	var groups [][]op
	var group []op
	for _, o := range ops {
		if o.tag == "equal" && o.az-o.ai > 6 {
			group = append(group, op{o.tag, o.ai, o.ai + 3, o.bi, o.bi + 3})
			groups = append(groups, group)
			group = nil
			o.ai = o.az - 3
			o.bi = o.bz - 3
		}
		group = append(group, o)
	}
	if len(group) > 0 && !(len(group) == 1 && group[0].tag == "equal") {
		groups = append(groups, group)
	}
	if len(groups) == 0 {
		return
	}
	fmt.Fprintf(out, "--- %s\n+++ %s\n", from, to)
	for _, g := range groups {
		f, l := g[0], g[len(g)-1]
		fmt.Fprintf(out, "@@ -%s +%s @@\n", span(f.ai, l.az), span(f.bi, l.bz))
		for _, o := range g {
			if o.tag == "equal" {
				for _, s := range a[o.ai:o.az] {
					fmt.Fprint(out, " "+s)
				}
			} else {
				if o.tag == "replace" || o.tag == "delete" {
					for _, s := range a[o.ai:o.az] {
						fmt.Fprint(out, "-"+s)
					}
				}
				if o.tag == "replace" || o.tag == "insert" {
					for _, s := range b[o.bi:o.bz] {
						fmt.Fprint(out, "+"+s)
					}
				}
			}
		}
	}
}

// Diff prints the live-to-desired diff, reversed for take; non-UTF8 bytes get a binary notice.
func Diff(out io.Writer, paths setup.Paths, p string, w setup.Want, reverse bool) error {
	var live []byte
	var e error
	if setup.IsLink(p) {
		t, e := os.Readlink(p)
		if e != nil {
			return e
		}
		live = []byte("-> " + t + "\n")
	} else if setup.IsFile(p) {
		live, e = os.ReadFile(p)
		if e != nil {
			return e
		}
	}
	next := w.Data
	if w.Kind == "link" {
		next = []byte("-> " + string(next) + "\n")
	}
	from, to := "live "+paths.Show(p), "dot "+paths.Show(p)
	if reverse {
		live, next = next, live
		from, to = to, from
	}
	if !utf8.Valid(live) || !utf8.Valid(next) {
		fmt.Fprintf(out, "binary files %s and %s differ\n", from, to)
		return nil
	}
	unified(out, lines(string(live)), lines(string(next)), from, to)
	return nil
}
