//go:build windows

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/archive"
	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/postgres"
	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/source"
	"github.com/pgdeploy/pgdeploy/internal/state"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
	"golang.org/x/sys/windows"
)

// Repair fixes verifiable parts of an owned instance: ACLs, service config,
// auto-start, recovery policy, and missing runtime files (when a trusted ZIP
// is available). It never runs initdb over an existing data directory.
func (in *InstallerImpl) Repair(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	r := Report{Root: root, ServiceName: service}
	release, err := lockInstance(root, service)
	if err != nil {
		return r, err
	}
	defer release()

	st, err := loadState(paths)
	if err != nil {
		return r, err
	}
	if st == nil {
		return r, diagnostics.RecoveryReview(
			"no install state at %s; cannot prove ownership, refusing to repair", installStatePath(paths))
	}
	if !strings.EqualFold(st.ServiceName, service) {
		return r, diagnostics.ServiceConflict("requested service does not match install state")
	}
	r.Port = st.Port
	r.DataDir = st.Data

	// Ensure the service exists (recreate only when it is missing and the
	// recorded ImagePath proves this instance owns the data).
	exists, err := in.SCM.Exists(service)
	if err != nil {
		return r, err
	}
	if !exists {
		if err := in.createServiceForState(st); err != nil {
			return r, err
		}
	} else {
		// Verify ownership before touching the service.
		spec, err := in.SCM.Config(service)
		if err != nil {
			return r, err
		}
		if !serviceConfigOwnedBy(spec, st) {
			return r, diagnostics.ServiceConflict(
				"service %q is not owned by this instance; refusing to modify it", service)
		}
	}

	// Re-apply and re-verify ACLs.
	serviceSID, err := security.ServiceSIDString(service)
	if err != nil {
		return r, diagnostics.Wrap(diagnostics.ACLFailed("cannot resolve service SID"), err)
	}

	// Correct auto-start + recovery policy.
	if err := in.fixServiceConfig(service, st); err != nil {
		return r, err
	}

	wasRunning := false
	if len(postgres.MissingRuntimeFiles(paths.RuntimeBin)) != 0 {
		svc, err := in.SCM.Query(service)
		if err != nil {
			return r, err
		}
		wasRunning = svc.State == winservice.StateRunning
		if svc.State != winservice.StateStopped {
			if err := in.SCM.Stop(service); err != nil {
				return r, err
			}
		}
	}

	// Restore missing runtime files from a trusted ZIP if available.
	if restored, err := in.restoreRuntime(ctx, paths, o); err != nil {
		return r, err
	} else if restored {
		r.Warnings = append(r.Warnings, "restored missing runtime files")
	}

	if err := in.applyACLsForPaths(paths, serviceSID); err != nil {
		return r, err
	}
	if wasRunning {
		if err := in.SCM.Start(service); err != nil {
			return r, err
		}
	}
	r.Status = StatusInstalled
	return r, nil
}

func (in *InstallerImpl) createServiceForState(st *state.InstallState) error {
	startType := winservice.StartAutomatic
	if st.ServiceStartType == "manual" {
		startType = winservice.StartManual
	}
	spec := winservice.ServiceSpec{
		Name:                st.ServiceName,
		DisplayName:         "PGDeploy PostgreSQL 18 (" + st.ServiceName + ")",
		BinaryPath:          filepath.Join(st.Runtime, "bin", "pg_ctl.exe"),
		Args:                []string{"runservice", "-N", st.ServiceName, "-D", st.Data, "-w"},
		ServiceStartName:    st.ServiceAccount,
		StartType:           startType,
		Dependencies:        []string{"RpcSs"},
		Description:         "PostgreSQL 18 managed by PGDeploy",
		SidTypeUnrestricted: true,
	}
	return in.SCM.Create(spec)
}

func serviceConfigOwnedBy(spec winservice.ServiceSpec, st *state.InstallState) bool {
	args := append([]string{spec.BinaryPath}, spec.Args...)
	if len(spec.Args) == 0 {
		var err error
		args, err = windows.DecomposeCommandLine(spec.BinaryPath)
		if err != nil {
			return false
		}
	}
	return serviceArgsOwnedBy(args, st)
}

func (in *InstallerImpl) applyACLsForPaths(paths state.Paths, serviceSID string) error {
	apply := []struct {
		path string
		role security.ACLRole
	}{
		{paths.Root, security.RoleRoot},
		{paths.State, security.RoleRoot},
		{filepath.Dir(paths.Runtime), security.RoleRoot},
		{paths.Runtime, security.RoleRuntime},
		{paths.Data, security.RoleData},
		{paths.Logs, security.RoleLogsDir},
		{paths.LogsPostgres, security.RoleLogsPostgres},
		{paths.Secrets, security.RoleSecrets},
	}
	for _, a := range apply {
		if err := security.ApplyDirACL(a.path, a.role, serviceSID); err != nil {
			return err
		}
		if err := security.VerifyDirACL(a.path, a.role, serviceSID); err != nil {
			return diagnostics.Wrap(diagnostics.ACLFailed("ACL verification failed"), err)
		}
	}
	return nil
}

func (in *InstallerImpl) fixServiceConfig(service string, st *state.InstallState) error {
	spec, err := in.SCM.Config(service)
	if err != nil {
		return err
	}
	wantAutomatic := st.ServiceStartType != "manual"
	gotAutomatic := spec.StartType == winservice.StartAutomatic
	if wantAutomatic != gotAutomatic {
		if wantAutomatic {
			if err := in.SCM.SetStartType(service, winservice.StartAutomatic); err != nil {
				return err
			}
		} else if err := in.SCM.SetStartType(service, winservice.StartManual); err != nil {
			return err
		}
	}
	return in.SCM.SetRecoveryActions(service)
}

// restoreRuntime re-extracts missing runtime files when a trusted ZIP is
// available in the executable directory or supplied via --zip.
func (in *InstallerImpl) restoreRuntime(ctx context.Context, paths state.Paths, o opts.Options) (bool, error) {
	missing := postgres.MissingRuntimeFiles(paths.RuntimeBin)
	if len(missing) == 0 {
		return false, nil
	}

	exe, err := os.Executable()
	if err != nil {
		return false, err
	}
	zipPath, err := archive.SelectZIP(filepath.Dir(exe), o.ZipPath)
	if err != nil {
		return false, err
	}
	ins, err := archive.Inspect(zipPath)
	if err != nil {
		return false, err
	}
	verdict := source.Verify(filepath.Base(zipPath), ins.SHA256, o.ExpectedSHA256)
	if !verdict.Verified {
		return false, diagnostics.SourceUnverified("runtime files are missing and no trusted ZIP is available to restore them")
	}

	staging, err := archive.NewStagingDir(filepath.Join(paths.Root, "staging"))
	if err != nil {
		return false, err
	}
	defer os.RemoveAll(staging)
	if err := security.ApplyDirACL(staging, security.RoleStaging, ""); err != nil {
		return false, err
	}
	destPgsql := filepath.Join(staging, "pgsql")
	if err := archive.Extract(zipPath, destPgsql); err != nil {
		return false, err
	}
	if err := postgres.CheckArchitecture(filepath.Join(destPgsql, "bin")); err != nil {
		return false, err
	}
	if _, err := postgres.Version(ctx, filepath.Join(destPgsql, "bin")); err != nil {
		return false, err
	}

	if err := os.MkdirAll(filepath.Dir(paths.Runtime), 0o700); err != nil {
		return false, err
	}
	if err := replaceRuntime(destPgsql, paths.Runtime); err != nil {
		return false, err
	}
	return true, nil
}
