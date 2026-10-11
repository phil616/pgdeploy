//go:build windows

package install

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/state"
	"github.com/pgdeploy/pgdeploy/internal/testutil"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
	"golang.org/x/sys/windows"
)

func uninstallFixture(t *testing.T) (*InstallerImpl, opts.Options) {
	t.Helper()
	root := filepath.Join(t.TempDir(), "instance")
	paths := state.DerivePaths(root)
	service := "PGDeployUninstallTest"
	st := &state.InstallState{Schema: state.SchemaVersion, Root: root, Runtime: paths.Runtime, Data: paths.Data, ServiceName: service, Port: 55432, Status: state.StatusInstalled}
	if err := state.Save(installStatePath(paths), st); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{paths.Runtime, paths.Data, paths.Secrets, paths.Logs, filepath.Join(root, "staging")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "fixture"), []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	scm := testutil.NewFakeServiceManager()
	if err := scm.Create(winservice.ServiceSpec{Name: service, BinaryPath: filepath.Join(paths.RuntimeBin, "pg_ctl.exe"), Args: []string{"runservice", "-N", service, "-D", paths.Data, "-w"}}); err != nil {
		t.Fatal(err)
	}
	return &InstallerImpl{SCM: scm}, opts.Options{RootPath: root, ServiceName: service}
}

func TestUninstallAlwaysRemovesRootAndService(t *testing.T) {
	for _, legacyPurge := range []bool{false, true} {
		in, o := uninstallFixture(t)
		if _, err := in.Uninstall(context.Background(), o, legacyPurge); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(o.RootPath); !os.IsNotExist(err) {
			t.Fatalf("root remains: %v", err)
		}
		if exists, err := in.SCM.Exists(o.ServiceName); err != nil || exists {
			t.Fatalf("service remains: %v", err)
		}
		if _, err := in.Uninstall(context.Background(), o, legacyPurge); err != nil {
			t.Fatalf("repeat uninstall: %v", err)
		}
	}
}

type cannotStopSCM struct{ winservice.ServiceManager }

func (s cannotStopSCM) Stop(string) error { return errors.New("stop failed") }

func TestUninstallKeepsFilesWhenStopFails(t *testing.T) {
	in, o := uninstallFixture(t)
	in.SCM = cannotStopSCM{in.SCM}
	if _, err := in.Uninstall(context.Background(), o, true); err == nil {
		t.Fatal("ignored stop failure")
	}
	if _, err := os.Stat(filepath.Join(o.RootPath, "data", "fixture")); err != nil {
		t.Fatal("removed data despite stop failure")
	}
	if exists, _ := in.SCM.Exists(o.ServiceName); !exists {
		t.Fatal("deleted service despite stop failure")
	}
}

func TestUninstallRejectsUnknownRoot(t *testing.T) {
	in, o := uninstallFixture(t)
	if err := os.Remove(filepath.Join(o.RootPath, "state", "install.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := in.Uninstall(context.Background(), o, true); err == nil {
		t.Fatal("removed unowned root")
	}
	if _, err := os.Stat(filepath.Join(o.RootPath, "data", "fixture")); err != nil {
		t.Fatal("removed unowned data")
	}
}

func TestUninstallCanRetryAfterLockedFile(t *testing.T) {
	in, o := uninstallFixture(t)
	path, err := windows.UTF16PtrFromString(filepath.Join(o.RootPath, "data", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateFile(path, windows.GENERIC_READ, windows.FILE_SHARE_READ, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if h != 0 {
			windows.CloseHandle(h)
		}
	}()
	if _, err := in.Uninstall(context.Background(), o, true); err == nil {
		t.Fatal("reported success with a locked file")
	}
	if st, err := loadState(state.DerivePaths(o.RootPath)); err != nil || st == nil {
		t.Fatalf("lost retry state: %v", err)
	}
	windows.CloseHandle(h)
	h = 0
	if _, err := in.Uninstall(context.Background(), o, true); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := os.Stat(o.RootPath); !os.IsNotExist(err) {
		t.Fatalf("root remains after retry: %v", err)
	}
}
