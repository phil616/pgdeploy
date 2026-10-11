package archive

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

func TestIsSupportedZipName(t *testing.T) {
	valid := []string{
		"postgresql-18.6-5-windows-x64-binaries.zip",
		"postgresql-18.6-5-windows-x64-binaries.ZIP",
		"postgresql-18.10-1-windows-x64-binaries.zip",
	}
	for _, n := range valid {
		if !IsSupportedZipName(n) {
			t.Errorf("IsSupportedZipName(%q) = false, want true", n)
		}
	}
	invalid := []string{
		"postgresql-17.2-1-windows-x64-binaries.zip",
		"postgresql-18.6-5-windows-x64-binaries.tar.gz",
		"random.zip",
		"postgresql-18.6-5-windows-x64-binaries.zip.exe",
	}
	for _, n := range invalid {
		if IsSupportedZipName(n) {
			t.Errorf("IsSupportedZipName(%q) = true, want false", n)
		}
	}
}

func writeFile(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestDiscoverZIPs(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip")
	writeFile(t, dir, "postgresql-17.0-0-windows-x64-binaries.zip")
	writeFile(t, dir, "notes.txt")
	if err := os.Mkdir(filepath.Join(dir, "postgresql-18.7-1-windows-x64-binaries.zip"), 0o755); err != nil {
		t.Fatal(err) // a directory that matches the name pattern should be ignored
	}

	got, err := DiscoverZIPs(dir)
	if err != nil {
		t.Fatalf("DiscoverZIPs: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("DiscoverZIPs returned %d entries, want 1: %v", len(got), got)
	}
	if filepath.Base(got[0]) != "postgresql-18.6-5-windows-x64-binaries.zip" {
		t.Errorf("unexpected match %q", got[0])
	}
}

func TestSelectZIP(t *testing.T) {
	t.Run("none found", func(t *testing.T) {
		dir := t.TempDir()
		_, err := SelectZIP(dir, "")
		if err == nil || diagnostics.ExitCode(err) != diagnostics.ExitZIP {
			t.Fatalf("expected E_ZIP_NOT_FOUND (exit %d), got %v", diagnostics.ExitZIP, err)
		}
	})

	t.Run("ambiguous", func(t *testing.T) {
		dir := t.TempDir()
		writeFile(t, dir, "postgresql-18.6-5-windows-x64-binaries.zip")
		writeFile(t, dir, "postgresql-18.7-1-windows-x64-binaries.zip")
		_, err := SelectZIP(dir, "")
		if err == nil || diagnostics.ExitCode(err) != diagnostics.ExitZIP {
			t.Fatalf("expected E_ZIP_AMBIGUOUS, got %v", err)
		}
	})

	t.Run("explicit invalid name", func(t *testing.T) {
		dir := t.TempDir()
		bad := filepath.Join(dir, "evil.zip")
		writeFile(t, dir, "evil.zip")
		_, err := SelectZIP(dir, bad)
		if err == nil || diagnostics.ExitCode(err) != diagnostics.ExitArchive {
			t.Fatalf("expected E_ZIP_INVALID, got %v", err)
		}
	})

	t.Run("explicit missing file", func(t *testing.T) {
		dir := t.TempDir()
		missing := filepath.Join(dir, "postgresql-18.6-5-windows-x64-binaries.zip")
		_, err := SelectZIP(dir, missing)
		if err == nil || diagnostics.ExitCode(err) != diagnostics.ExitZIP {
			t.Fatalf("expected E_ZIP_NOT_FOUND, got %v", err)
		}
	})

	t.Run("explicit valid", func(t *testing.T) {
		dir := t.TempDir()
		zip := "postgresql-18.6-5-windows-x64-binaries.zip"
		writeFile(t, dir, zip)
		got, err := SelectZIP(dir, filepath.Join(dir, zip))
		if err != nil {
			t.Fatalf("SelectZIP: %v", err)
		}
		if filepath.Base(got) != zip {
			t.Errorf("got %q, want %q", got, zip)
		}
	})
}
