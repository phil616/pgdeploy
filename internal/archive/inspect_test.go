package archive

import (
	"strings"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/testutil"
)

func TestNormalizeEntryName(t *testing.T) {
	valid := map[string]string{
		"bin/postgres.exe":       "bin/postgres.exe",
		"pgsql/bin/postgres.exe": "pgsql/bin/postgres.exe",
		`bin\postgres.exe`:       "bin/postgres.exe", // backslashes are separators
		"share/":                 "share/",
	}
	for in, want := range valid {
		got, err := normalizeEntryName(in)
		if err != nil {
			t.Errorf("normalizeEntryName(%q): unexpected error %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("normalizeEntryName(%q) = %q, want %q", in, got, want)
		}
	}

	rejected := []string{
		"/etc/passwd",      // absolute path
		"C:/evil.exe",      // drive letter
		`\\server\share\x`, // UNC
		"a:b.exe",          // colon / ADS
		"../evil.exe",      // traversal
		"a/../../evil.exe", // nested traversal
		"CON",              // reserved device name
		"con.txt",          // reserved device name with extension
		"NUL", "AUX", "PRN", "COM1", "LPT9",
		"trailing.", // trailing dot
		"trailing ", // trailing space
	}
	for _, in := range rejected {
		if got, err := normalizeEntryName(in); err == nil {
			t.Errorf("normalizeEntryName(%q) = %q, want error", in, got)
		}
	}
}

func TestIsReservedDeviceName(t *testing.T) {
	for _, n := range []string{"CON", "con", "con.txt", "PRN", "AUX", "NUL", "COM1", "com9", "LPT1", "lpt9"} {
		if !isReservedDeviceName(n) {
			t.Errorf("isReservedDeviceName(%q) = false, want true", n)
		}
	}
	for _, n := range []string{"console", "COM10", "file.txt", "postgres.exe", ""} {
		if isReservedDeviceName(n) {
			t.Errorf("isReservedDeviceName(%q) = true, want false", n)
		}
	}
}

func validArchiveEntries() []testutil.ZipEntry {
	return []testutil.ZipEntry{
		{Name: "pgsql/bin/postgres.exe", Content: "x"},
		{Name: "pgsql/bin/initdb.exe", Content: "x"},
		{Name: "pgsql/bin/pg_ctl.exe", Content: "x"},
		{Name: "pgsql/bin/pg_isready.exe", Content: "x"},
		{Name: "pgsql/bin/psql.exe", Content: "x"},
		{Name: "pgsql/share", Dir: true},
		{Name: "pgsql/lib", Dir: true},
	}
}

func TestInspectValidArchive(t *testing.T) {
	dir := t.TempDir()
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", validArchiveEntries())

	ins, err := Inspect(zipPath)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if ins.TopLevelPrefix != "pgsql/" {
		t.Errorf("TopLevelPrefix = %q, want %q", ins.TopLevelPrefix, "pgsql/")
	}
	if ins.EntryCount != 7 {
		t.Errorf("EntryCount = %d, want 7", ins.EntryCount)
	}
	if len(ins.SHA256) != 64 {
		t.Errorf("SHA256 length = %d, want 64", len(ins.SHA256))
	}
}

func TestInspectMissingRequiredFile(t *testing.T) {
	dir := t.TempDir()
	entries := validArchiveEntries()
	// Drop initdb.exe.
	entries = append(entries[:1], entries[2:]...)
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", entries)

	if _, err := Inspect(zipPath); err == nil {
		t.Fatal("Inspect succeeded despite a missing required file")
	}
}

func TestInspectDuplicateCaseCollision(t *testing.T) {
	dir := t.TempDir()
	entries := validArchiveEntries()
	// Add a case-colliding duplicate of postgres.exe.
	entries = append(entries, testutil.ZipEntry{Name: "pgsql/bin/POSTGRES.EXE", Content: "y"})
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", entries)

	if _, err := Inspect(zipPath); err == nil {
		t.Fatal("Inspect succeeded despite a case-colliding duplicate entry")
	}
}

func TestInspectRejectsSymlink(t *testing.T) {
	dir := t.TempDir()
	entries := validArchiveEntries()
	entries = append(entries, testutil.ZipEntry{Name: "pgsql/bin/link", Symlink: true})
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", entries)

	_, err := Inspect(zipPath)
	if err == nil {
		t.Fatal("Inspect succeeded despite a symlink entry")
	}
	if !strings.Contains(err.Error(), "not a regular file") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestComputeSHA256(t *testing.T) {
	dir := t.TempDir()
	zipPath := testutil.WriteTestZIP(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip", validArchiveEntries())
	h, err := ComputeSHA256(zipPath)
	if err != nil {
		t.Fatalf("ComputeSHA256: %v", err)
	}
	if len(h) != 64 {
		t.Errorf("SHA256 length = %d, want 64", len(h))
	}
}

func TestInspectImplicitDirectoriesAndTopLevelReadme(t *testing.T) {
	entries := validArchiveEntries()[:5]
	entries = append(entries,
		testutil.ZipEntry{Name: "pgsql/share/postgres.bki", Content: "bki"},
		testutil.ZipEntry{Name: "pgsql/lib/libpq.dll", Content: "dll"},
		testutil.ZipEntry{Name: "README.txt", Content: "archive notes"},
	)
	zipPath := testutil.WriteTestZIP(t, t.TempDir(), "archive.zip", entries)
	for i := 0; i < 20; i++ {
		ins, err := Inspect(zipPath)
		if err != nil {
			t.Fatal(err)
		}
		if ins.TopLevelPrefix != "pgsql/" {
			t.Fatalf("unexpected prefix %q", ins.TopLevelPrefix)
		}
	}
}
