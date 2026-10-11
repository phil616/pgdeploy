package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/testutil"
)

func TestExtractStripsPrefix(t *testing.T) {
	dir := t.TempDir()
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", []testutil.ZipEntry{
		{Name: "pgsql/bin/postgres.exe", Content: "bin"},
		{Name: "pgsql/bin/initdb.exe", Content: "init"},
		{Name: "pgsql/bin/pg_ctl.exe", Content: "ctl"},
		{Name: "pgsql/bin/pg_isready.exe", Content: "ready"},
		{Name: "pgsql/bin/psql.exe", Content: "psql"},
		{Name: "pgsql/share", Dir: true},
		{Name: "pgsql/lib", Dir: true},
	})

	dest := filepath.Join(dir, "out")
	if err := Extract(zipPath, dest); err != nil {
		t.Fatalf("Extract: %v", err)
	}

	// The pgsql/ prefix must be stripped so binaries land directly under dest.
	for _, f := range []string{"bin/postgres.exe", "bin/initdb.exe", "bin/pg_ctl.exe", "bin/pg_isready.exe", "bin/psql.exe"} {
		if _, err := os.Stat(filepath.Join(dest, filepath.FromSlash(f))); err != nil {
			t.Errorf("expected %s to exist after extraction: %v", f, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "pgsql")); err == nil {
		t.Error("pgsql/ prefix was not stripped during extraction")
	}
}

func TestExtractRefusesExistingDest(t *testing.T) {
	dir := t.TempDir()
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", validArchiveEntries())
	dest := filepath.Join(dir, "out")
	if err := os.Mkdir(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Extract(zipPath, dest); err == nil {
		t.Fatal("Extract succeeded into an existing directory")
	}
}
