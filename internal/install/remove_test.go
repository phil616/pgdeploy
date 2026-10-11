package install

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRemoveEntireInstance(t *testing.T) {
	parent := t.TempDir()
	root := filepath.Join(parent, "instance")
	for _, name := range []string{"runtime/pgsql/bin/postgres.exe", "data/base/database", "logs/installer.jsonl", "secrets/postgres-password.txt", "state/install.json", "staging/file", "backup.dump"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	neighbor := filepath.Join(parent, "other-instance")
	if err := os.Mkdir(neighbor, 0700); err != nil {
		t.Fatal(err)
	}
	if err := removeInstanceFiles(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatalf("root remains: %v", err)
	}
	if _, err := os.Stat(neighbor); err != nil {
		t.Fatalf("removed neighboring instance: %v", err)
	}
}

func TestRemovalDoesNotFollowSymlink(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	file := filepath.Join(outside, "keep.txt")
	if err := os.WriteFile(file, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "external")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := removeInstanceFiles(root); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(file); err != nil {
		t.Fatalf("followed symlink: %v", err)
	}
}
