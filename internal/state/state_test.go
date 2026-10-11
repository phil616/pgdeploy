package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "install.json")
	st := &InstallState{
		ServiceName:     "PGDeployPostgreSQL18",
		ServiceAccount:  `NT SERVICE\PGDeployPostgreSQL18`,
		Root:            `C:\ProgramData\PGDeploy`,
		Port:            5432,
		PostgresVersion: "18.6",
		Status:          StatusInstalled,
	}
	if err := Save(p, st); err != nil {
		t.Fatalf("Save: %v", err)
	}
	got, err := Load(p)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got == nil {
		t.Fatal("Load returned nil")
	}
	if got.Schema != SchemaVersion {
		t.Errorf("Schema = %d, want %d", got.Schema, SchemaVersion)
	}
	if got.ServiceName != st.ServiceName || got.Port != st.Port || got.Status != st.Status {
		t.Errorf("round-trip mismatch: %+v", got)
	}
}

func TestLoadMissingFile(t *testing.T) {
	got, err := Load(filepath.Join(t.TempDir(), "install.json"))
	if err != nil {
		t.Fatalf("Load missing file should return nil error, got %v", err)
	}
	if got != nil {
		t.Fatalf("Load missing file should return nil state, got %+v", got)
	}
}

func TestLoadCorruptFile(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "install.json")
	if err := os.WriteFile(p, []byte("{not valid json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("Load corrupt file should error")
	}
}

func TestLoadWrongSchema(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "install.json")
	if err := os.WriteFile(p, []byte(`{"schema": 99}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("Load unsupported schema should error")
	}
}

func TestDerivePaths(t *testing.T) {
	p := DerivePaths(`C:\ProgramData\PGDeploy\instances\PGDeployPostgreSQL18`)
	if p.Root != `C:\ProgramData\PGDeploy\instances\PGDeployPostgreSQL18` {
		t.Errorf("Root = %q", p.Root)
	}
	if p.Runtime != filepath.Join(p.Root, "runtime", "pgsql") {
		t.Errorf("Runtime = %q", p.Runtime)
	}
	if p.Data != filepath.Join(p.Root, "data") {
		t.Errorf("Data = %q", p.Data)
	}
	if p.Secrets != filepath.Join(p.Root, "secrets") {
		t.Errorf("Secrets = %q", p.Secrets)
	}
}
