package app

// This internal test scripts the terminal gate, which the public surface keeps unexported so
// the updater's only test seam is whether dot is talking to a person. It builds its own paths:
// testutil would be an import cycle here.

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fschrhunt/dot/internal/setup"
)

// TestNoticeNudgesOnceADay pins the terminal notice: a stale check with a newer release prints
// how to update, and the remembered check keeps the second call silent.
func TestNoticeNudgesOnceADay(t *testing.T) {
	old := terminal
	terminal = func(*os.File) bool { return true }
	defer func() { terminal = old }()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/tag/v9.9.9", http.StatusFound)
			return
		}
		http.NotFound(w, r)
	}))
	defer s.Close()
	t.Setenv("DOT_RELEASES", s.URL)
	tmp := t.TempDir()
	paths := setup.Paths{Home: tmp, Dot: tmp, State: filepath.Join(tmp, ".state"), Machine: "laptop"}
	stale, _ := json.Marshal(checked{At: time.Now().Add(-48 * time.Hour), Latest: "v0.0.0"})
	if e := os.MkdirAll(paths.State, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(paths.State, "update-check"), stale, 0600); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	Notice(paths, "v0.0.0", &b)
	if got := b.String(); got != "\ndot v9.9.9 is out (you have v0.0.0): dot update\n" {
		t.Fatalf("got %q", got)
	}
	b.Reset()
	Notice(paths, "v0.0.0", &b)
	if b.String() != "" {
		t.Fatalf("the remembered check spoke again: %q", b.String())
	}
}

// TestNoticeStaysSilentWithoutATerminal pins the gate: a stale check still speaks to nobody in
// a pipe, and asks the network nothing.
func TestNoticeStaysSilentWithoutATerminal(t *testing.T) {
	hit := false
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hit = true
		http.NotFound(w, r)
	}))
	defer s.Close()
	t.Setenv("DOT_RELEASES", s.URL)
	tmp := t.TempDir()
	paths := setup.Paths{Home: tmp, Dot: tmp, State: filepath.Join(tmp, ".state"), Machine: "laptop"}
	stale, _ := json.Marshal(checked{At: time.Now().Add(-48 * time.Hour), Latest: "v0.0.0"})
	if e := os.MkdirAll(paths.State, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(paths.State, "update-check"), stale, 0600); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	Notice(paths, "v0.0.0", &b)
	if b.String() != "" {
		t.Fatalf("spoke without a terminal: %q", b.String())
	}
	if hit {
		t.Fatal("asked the network without a terminal")
	}
}
