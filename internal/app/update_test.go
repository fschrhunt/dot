package app_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/fschrhunt/dot/internal/app"
	"github.com/fschrhunt/dot/internal/testutil"
)

// releases serves a fake releases root whose latest is v9.9.9, and fails the test on any
// unexpected request when strict.
func releases(t *testing.T, strict bool) string {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/latest" {
			http.Redirect(w, r, "/tag/v9.9.9", http.StatusFound)
			return
		}
		if strict {
			t.Error("unexpected request to " + r.URL.Path)
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(s.Close)
	t.Setenv("DOT_RELEASES", s.URL)
	return s.URL
}

// TestUpdateCheckReportsANewerRelease pins the command: --check names the release and how to
// get it, exiting 1 while one is out.
func TestUpdateCheckReportsANewerRelease(t *testing.T) {
	f := testutil.New(t)
	releases(t, false)
	var b bytes.Buffer
	code, e := app.Update(f.Paths, "v0.0.0", true, &b)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 1)
	testutil.Equal(t, b.String(), "dot: v9.9.9 is out (you have v0.0.0); update with: dot update\n")
}

// TestUpdateIsQuietWhenCurrent pins the command: an up-to-date binary says so and exits 0.
func TestUpdateIsQuietWhenCurrent(t *testing.T) {
	f := testutil.New(t)
	releases(t, false)
	var b bytes.Buffer
	code, e := app.Update(f.Paths, "v9.9.9", false, &b)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 0)
	testutil.Equal(t, b.String(), "dot: v9.9.9 is the latest\n")
}

// TestUpdateRefusesADevelopmentBuild pins the command: a dev binary is updated from source,
// without ever asking the network.
func TestUpdateRefusesADevelopmentBuild(t *testing.T) {
	f := testutil.New(t)
	t.Setenv("DOT_RELEASES", "http://127.0.0.1:1/")
	var b bytes.Buffer
	code, e := app.Update(f.Paths, "dev", false, &b)
	if e != nil {
		t.Fatal(e)
	}
	testutil.Equal(t, code, 1)
	if got := b.String(); got != "dot: this is a development build (dev); update it from source\n" {
		t.Fatal(got)
	}
}

// TestNoticeStaysSilentOnAFreshCheck pins the once-a-day record: a fresh check speaks to
// nobody and asks the network nothing.
func TestNoticeStaysSilentOnAFreshCheck(t *testing.T) {
	f := testutil.New(t)
	releases(t, true)
	remembered, _ := json.Marshal(map[string]any{"at": time.Now(), "latest": "v9.9.9"})
	if e := os.MkdirAll(f.Paths.State, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(f.Paths.State, "update-check"), remembered, 0600); e != nil {
		t.Fatal(e)
	}
	var b bytes.Buffer
	app.Notice(f.Paths, "v0.0.0", &b)
	testutil.Equal(t, b.String(), "")
}
