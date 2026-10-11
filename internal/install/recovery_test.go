package install

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

func TestPasswordSurvivesRetry(t *testing.T) {
	dir := t.TempDir()
	pw, err := acquirePassword("", dir, false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := security.WriteSecretFile(dir, "postgres-password.txt", []byte(pw+"\n")); err != nil {
		t.Fatal(err)
	}
	for _, initialized := range []bool{false, true} {
		got, err := acquirePassword("", dir, initialized)
		if err != nil || got != pw {
			t.Fatalf("retry initialized=%v changed password: %v", initialized, err)
		}
	}
	input := filepath.Join(t.TempDir(), "password.txt")
	if err := os.WriteFile(input, []byte("different"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := acquirePassword(input, dir, true); err == nil {
		t.Fatal("accepted a different password for existing cluster")
	}
}

func TestExistingClusterRequiresPassword(t *testing.T) {
	if _, err := acquirePassword("", t.TempDir(), true); err == nil {
		t.Fatal("generated password for existing cluster")
	}
}

func TestRuntimeReplacementRollback(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "runtime")
	if err := os.Mkdir(target, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "old"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := replaceRuntime(filepath.Join(root, "missing"), target); err == nil {
		t.Fatal("expected failed promotion")
	}
	if _, err := os.Stat(filepath.Join(target, "old")); err != nil {
		t.Fatalf("lost original runtime: %v", err)
	}
	staged := filepath.Join(root, "staged")
	if err := os.Mkdir(staged, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staged, "new"), []byte("new"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := replaceRuntime(staged, target); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "new")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(target, "old")); !os.IsNotExist(err) {
		t.Fatal("old runtime still present")
	}
}

func TestServiceOwnershipRequiresExactPaths(t *testing.T) {
	st := &state.InstallState{ServiceName: "TestPG", Runtime: filepath.Join(t.TempDir(), "runtime"), Data: filepath.Join(t.TempDir(), "data")}
	args := []string{filepath.Join(st.Runtime, "bin", "pg_ctl.exe"), "runservice", "-N", st.ServiceName, "-D", st.Data, "-w"}
	if !serviceArgsOwnedBy(args, st) {
		t.Fatal("rejected owned service")
	}
	for _, index := range []int{0, 3, 5} {
		bad := append([]string(nil), args...)
		bad[index] += "-other"
		if serviceArgsOwnedBy(bad, st) {
			t.Fatalf("accepted substring at argument %d", index)
		}
	}
}
