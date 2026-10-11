// Package testutil provides shared test doubles and fixtures for unit tests.
package testutil

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"
)

// ZipEntry describes a single file, directory, or symlink in a test archive.
type ZipEntry struct {
	Name    string
	Content string
	Dir     bool
	Symlink bool
}

// WriteTestZIP builds a ZIP archive at dir/name containing entries and returns
// its absolute path. Entry names are written verbatim, which lets callers
// craft archives for zip-slip and structure-validation tests.
func WriteTestZIP(t *testing.T, dir, name string, entries []ZipEntry) string {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, name)
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	zw := zip.NewWriter(f)
	for _, e := range entries {
		hdr := &zip.FileHeader{Name: e.Name, Method: zip.Deflate}
		switch {
		case e.Dir:
			hdr.Name = e.Name + "/"
			hdr.SetMode(os.ModeDir | 0o755)
		case e.Symlink:
			hdr.SetMode(os.ModeSymlink | 0o777)
		default:
			hdr.SetMode(0o644)
		}
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			t.Fatalf("create header for %q: %v", e.Name, err)
		}
		if !e.Dir && !e.Symlink {
			if _, err := w.Write([]byte(e.Content)); err != nil {
				t.Fatalf("write %q: %v", e.Name, err)
			}
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return p
}
