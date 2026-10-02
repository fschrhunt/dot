package update

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestReleaseTagsAndNewerOrdersThem pins version parsing: vX.Y.Z compares by numbers.
func TestReleaseTagsAndNewerOrdersThem(t *testing.T) {
	for _, v := range []string{"v1.0.0", "v1.2.3", "v10.0.0"} {
		if !Release(v) {
			t.Fatal(v + " is not a release")
		}
	}
	for _, v := range []string{"dev", "1.0.0", "v1.0", "v1.0.0.0", "v1.-1.0", ""} {
		if Release(v) {
			t.Fatal(v + " is a release")
		}
	}
	for _, c := range [][3]string{{"v1.0.1", "v1.0.0", ">"}, {"v1.0.0", "v1.0.1", "<"}, {"v2.0.0", "v10.0.0", "<"}, {"v1.0.0", "v1.0.0", "="}} {
		newer := Newer(c[0], c[1])
		if (c[2] == ">") != newer || c[2] == "=" && newer {
			t.Fatalf("Newer(%s, %s) = %v", c[0], c[1], newer)
		}
	}
}

// TestUpdateHintsPointAtTheirUpdaters pins the install-method routing: only a direct install
// updates itself.
func TestUpdateHintsPointAtTheirUpdaters(t *testing.T) {
	for method, hint := range map[string]string{"brew": "brew upgrade dot", "go": "go install github.com/fschrhunt/dot/cmd/dot@latest", "direct": "dot update"} {
		if Hint(method) != hint {
			t.Fatalf("Hint(%s) = %q", method, Hint(method))
		}
	}
}

// standin serves a fake releases root: /latest redirects to the tag, and the tag's archive
// and checksums follow. checksums starts valid; tests rewrite it to break the verification.
func standin(t *testing.T, tag string, archive []byte) (*httptest.Server, *string) {
	t.Helper()
	name := fmt.Sprintf("dot_%s_%s_%s.tar.gz", tag, runtime.GOOS, runtime.GOARCH)
	sum := sha256.Sum256(archive)
	checksums := hex.EncodeToString(sum[:]) + "  " + name + "\n"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/latest":
			http.Redirect(w, r, "/tag/"+tag, http.StatusFound)
		case "/download/" + tag + "/" + name:
			w.Write(archive)
		case "/download/" + tag + "/checksums.txt":
			w.Write([]byte(checksums))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(s.Close)
	t.Setenv("DOT_RELEASES", s.URL)
	return s, &checksums
}

// pack builds an archive holding one regular file, as the release workflow makes it.
func pack(t *testing.T, name string, data []byte) []byte {
	t.Helper()
	var archive bytes.Buffer
	z := gzip.NewWriter(&archive)
	w := tar.NewWriter(z)
	if e := w.WriteHeader(&tar.Header{Name: name, Mode: 0755, Size: int64(len(data)), Typeflag: tar.TypeReg}); e != nil {
		t.Fatal(e)
	}
	if _, e := w.Write(data); e != nil {
		t.Fatal(e)
	}
	if e := w.Close(); e != nil {
		t.Fatal(e)
	}
	if e := z.Close(); e != nil {
		t.Fatal(e)
	}
	return archive.Bytes()
}

// TestLatestReadsTheRedirectedTag pins latest-version resolution without the network.
func TestLatestReadsTheRedirectedTag(t *testing.T) {
	standin(t, "v9.9.9", pack(t, "dot", []byte("x")))
	latest, e := Latest(5 * time.Second)
	if e != nil {
		t.Fatal(e)
	}
	if latest != "v9.9.9" {
		t.Fatal(latest)
	}
}

// TestApplyToReplacesTheBinaryAfterCheckingIt pins the self-update: the file at exe becomes
// the archive's binary, with nothing left beside it.
func TestApplyToReplacesTheBinaryAfterCheckingIt(t *testing.T) {
	standin(t, "v9.9.9", pack(t, "dot", []byte("new")))
	exe := filepath.Join(t.TempDir(), "dot")
	if e := os.WriteFile(exe, []byte("old"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := ApplyTo(exe, "v9.9.9"); e != nil {
		t.Fatal(e)
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != "new" {
		t.Fatal(string(b))
	}
	if _, e := os.Stat(exe + ".next"); !os.IsNotExist(e) {
		t.Fatal("the swap left .dot.next behind")
	}
}

// TestApplyToRefusesAChecksumMismatch pins the seat belt: a bad archive changes nothing.
func TestApplyToRefusesAChecksumMismatch(t *testing.T) {
	_, checksums := standin(t, "v9.9.9", pack(t, "dot", []byte("new")))
	*checksums = "0000  dot_v9.9.9_" + runtime.GOOS + "_" + runtime.GOARCH + ".tar.gz\n"
	exe := filepath.Join(t.TempDir(), "dot")
	if e := os.WriteFile(exe, []byte("old"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := ApplyTo(exe, "v9.9.9"); e == nil {
		t.Fatal("a tampered archive applied")
	}
	b, e := os.ReadFile(exe)
	if e != nil {
		t.Fatal(e)
	}
	if string(b) != "old" {
		t.Fatal("a tampered archive replaced the binary")
	}
}

// TestApplyToRefusesAnArchiveWithoutTheBinary pins the archive shape: LICENSE alone is not dot.
func TestApplyToRefusesAnArchiveWithoutTheBinary(t *testing.T) {
	standin(t, "v9.9.9", pack(t, "LICENSE", []byte("MIT")))
	exe := filepath.Join(t.TempDir(), "dot")
	if e := os.WriteFile(exe, []byte("old"), 0755); e != nil {
		t.Fatal(e)
	}
	if e := ApplyTo(exe, "v9.9.9"); e == nil {
		t.Fatal("an archive without dot applied")
	}
}

// TestPipesAreNotTerminals pins the terminal gate on both systems: the updater never nudges a pipe.
func TestPipesAreNotTerminals(t *testing.T) {
	r, w, e := os.Pipe()
	if e != nil {
		t.Fatal(e)
	}
	defer r.Close()
	defer w.Close()
	if Terminal(r) || Terminal(w) {
		t.Fatal("a pipe reports as a terminal")
	}
	if f, e := os.Open(os.DevNull); e != nil {
		t.Fatal(e)
	} else {
		defer f.Close()
		if Terminal(f) {
			t.Fatal("/dev/null reports as a terminal")
		}
	}
}
