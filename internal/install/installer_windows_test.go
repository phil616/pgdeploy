//go:build windows

package install

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/state"
	"github.com/pgdeploy/pgdeploy/internal/testutil"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

func TestInstallAlreadyInstalledWithoutZIP(t *testing.T) {
	root := t.TempDir()
	paths := state.DerivePaths(root)
	service := "PGDeployIdempotencyTest"
	st := &state.InstallState{Schema: state.SchemaVersion, Root: root, Runtime: paths.Runtime, Data: paths.Data, ServiceName: service, Port: 55432, Status: state.StatusInstalled}
	if err := state.Save(installStatePath(paths), st); err != nil {
		t.Fatal(err)
	}
	scm := testutil.NewFakeServiceManager()
	if err := scm.Create(winservice.ServiceSpec{Name: service, BinaryPath: filepath.Join(paths.RuntimeBin, "pg_ctl.exe"), Args: []string{"runservice", "-N", service, "-D", paths.Data, "-w"}}); err != nil {
		t.Fatal(err)
	}
	if err := scm.Start(service); err != nil {
		t.Fatal(err)
	}
	in := &InstallerImpl{SCM: scm}
	o := opts.Options{RootPath: root, ServiceName: service, Port: 55432}
	rep, err := in.Install(context.Background(), o)
	if err != nil || !rep.AlreadyInstalled {
		t.Fatalf("idempotent install: %+v %v", rep, err)
	}
	o.Port++
	if _, err := in.Install(context.Background(), o); err == nil {
		t.Fatal("accepted conflicting instance options")
	}
}

func TestSCMCommandLineOwnership(t *testing.T) {
	st := &state.InstallState{Runtime: `C:\Customer Apps\runtime\pgsql`, Data: `C:\Customer Apps\data`, ServiceName: "CustomerPG"}
	spec := winservice.ServiceSpec{BinaryPath: `"C:\Customer Apps\runtime\pgsql\bin\pg_ctl.exe" runservice -N CustomerPG -D "C:\Customer Apps\data" -w`}
	if !serviceConfigOwnedBy(spec, st) {
		t.Fatal("rejected quoted SCM command")
	}
	spec.BinaryPath = `"C:\Customer Apps\runtime\pgsql\bin\pg_ctl.exe" runservice -N CustomerPG -D "C:\Customer Apps\data-other" -w`
	if serviceConfigOwnedBy(spec, st) {
		t.Fatal("accepted prefix collision")
	}
}
