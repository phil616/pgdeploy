//go:build windows

package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/archive"
	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/plan"
	"github.com/pgdeploy/pgdeploy/internal/postgres"
	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/source"
	"github.com/pgdeploy/pgdeploy/internal/state"
	"github.com/pgdeploy/pgdeploy/internal/version"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

// Installer is the concrete implementation.
type InstallerImpl struct {
	SCM      winservice.ServiceManager
	Elevator winservice.Elevator
}

// New returns an installer using the real SCM and elevator.
func New() *InstallerImpl {
	return &InstallerImpl{SCM: winservice.New(), Elevator: winservice.NewElevator()}
}

func installStatePath(paths state.Paths) string { return filepath.Join(paths.State, "install.json") }

// Verify runs the read-only checks without writing a service or database.
func (in *InstallerImpl) Verify(ctx context.Context, o opts.Options) (Report, error) {
	p, err := plan.Resolve(o)
	if err != nil {
		return Report{}, err
	}
	r := Report{Root: p.Root, ServiceName: p.ServiceName, Port: p.Port, DataDir: p.Paths.Data}

	verdict := source.Verify(filepath.Base(p.ZipPath), p.Inspection.SHA256, o.ExpectedSHA256)
	if !verdict.Verified {
		if verdict.Label != diagnostics.WarnSourceAuthenticityUnverified {
			return r, source.ErrForInstall(verdict)
		}
		r.Status = StatusUnverified
		r.Code = verdict.Label
		r.Warnings = append(r.Warnings, verdict.Reason)
		return r, nil
	}
	r.Status = StatusVerified
	r.Warnings = append(r.Warnings, verdict.Label)
	return r, nil
}

// Install performs the full installation transaction.
func (in *InstallerImpl) Install(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	release, err := lockInstance(root, service)
	if err != nil {
		return Report{}, err
	}
	defer release()
	r := Report{Root: root, ServiceName: service, Port: o.Port, DataDir: paths.Data}
	st, err := loadState(paths)
	if err != nil {
		return r, err
	}
	if st != nil {
		if !strings.EqualFold(st.ServiceName, service) || !strings.EqualFold(st.Root, root) || st.Port != o.Port {
			return r, diagnostics.RecoveryReview("installation options conflict with the recorded instance")
		}
		if st.Status == state.StatusInstalled || st.Status == state.StatusInstalledNotStart {
			spec, err := in.SCM.Config(service)
			if err != nil {
				return r, err
			}
			if !serviceConfigOwnedBy(spec, st) {
				return r, diagnostics.ServiceConflict("service %q is not owned by this instance", service)
			}
			r.Status = StatusAlreadyInstalled
			r.AlreadyInstalled = true
			r.PostgresVersion = st.PostgresVersion
			r.ConnectionURL = connectionURL(st.Port)
			r.CredentialsHint = credentialsHint(root)
			return r, nil
		}
	}
	p, err := plan.Resolve(o)
	if err != nil {
		return r, err
	}
	verdict := source.Verify(filepath.Base(p.ZipPath), p.Inspection.SHA256, o.ExpectedSHA256)
	if !verdict.Verified {
		return r, source.ErrForInstall(verdict)
	}
	if owned, err := in.serviceOwnedByUs(p); err != nil {
		return r, err
	} else if !owned {
		return r, diagnostics.ServiceConflict("service %q is not managed by this installation", service)
	}
	// A resumed, running service legitimately owns the port.
	if exists, err := in.SCM.Exists(service); err != nil {
		return r, err
	} else if !exists {
		if err := plan.CheckPort(p.Port); err != nil {
			return r, err
		}
	}

	// Logging + journal.
	if err := os.MkdirAll(p.Paths.Logs, 0o700); err != nil {
		return r, err
	}
	txid := NewTxID()
	log, err := diagnostics.OpenLogger(p.Paths.Logs, txid, o.Verbose, os.Stderr)
	if err != nil {
		return r, err
	}
	defer log.Close()
	tx, err := OpenTransaction(p.Paths, txid, log)
	if err != nil {
		return r, err
	}

	log.Infof("preflight", "plan resolved", "root=%s service=%s port=%d zip=%s", p.Root, p.ServiceName, p.Port, filepath.Base(p.ZipPath))
	log.Infof("archive.verify", "sha256", "%s", p.Inspection.SHA256)
	if !p.HasEnoughSpace() {
		return r, diagnostics.FSUnsupported(
			"insufficient free space on %s: need %d bytes, have %d", volumeName(p.Root), p.RequiredSpace(), p.FreeBytes)
	}

	// Determine resume phase and whether the data directory is already ours.
	phase := tx.Journal.LastPhase()
	dataInitialized := fileExists(filepath.Join(p.Paths.Data, "PG_VERSION"))
	if dataInitialized && state.PhaseOrder(phase) < state.PhaseOrder(state.PhaseDataInitializing) {
		return r, diagnostics.DataDirOccupied(
			"data directory %s already contains a database but has no PGDeploy ownership record; refusing to initialize or delete it", p.Paths.Data)
	}

	// 1. Directory tree + early secrets ACL.
	if err := in.ensureTree(p); err != nil {
		return r, err
	}
	if st == nil {
		if err := in.commitState(p, verdict.Label, "", state.StatusPartial); err != nil {
			return r, err
		}
	}
	if err := tx.Advance(&phase, state.PhasePreflightOK, "preflight complete"); err != nil {
		return r, err
	}

	// 2. Extract + validate runtime (skipped if already complete).
	if err := in.ensureRuntime(ctx, p, tx, &phase); err != nil {
		return r, err
	}

	// initdb disables the Administrators SID. Grant its user SID explicitly
	// during bootstrap; final ACLs below remove these temporary grants.
	defer security.ApplyDirACL(p.Paths.Secrets, security.RoleSecrets, "")
	if !dataInitialized {
		callerSID, err := security.CurrentUserSIDString()
		if err != nil {
			return r, err
		}
		for _, a := range []struct {
			path string
			role security.ACLRole
		}{
			{p.Paths.Root, security.RoleRoot},
			{filepath.Dir(p.Paths.Runtime), security.RoleRoot},
			{p.Paths.Runtime, security.RoleRuntime},
			{p.Paths.Data, security.RoleData},
			{p.Paths.Secrets, security.RoleData},
		} {
			if err := security.ApplyDirACL(a.path, a.role, callerSID); err != nil {
				return r, err
			}
		}

	}

	// 3. Password.
	password, err := acquirePassword(o.PasswordFile, p.Paths.Secrets, dataInitialized)
	if err != nil {
		return r, err
	}

	// Persist before initdb: every retry must use the same database password.
	if err := in.writeFinalPassword(p, password); err != nil {
		return r, err
	}

	// 4. initdb (skipped if data already initialized by a prior run).
	if !dataInitialized {
		if err := in.initDB(ctx, p, tx, &phase, password); err != nil {
			return r, err
		}
	}

	if err := postgres.ValidateDataDirectory(p.Paths.Data); err != nil {
		return r, err
	}
	if err := in.writeConfig(p); err != nil {
		return r, err
	}

	// 5. Create service (don't start yet).
	if exists, err := in.SCM.Exists(p.ServiceName); err != nil {
		return r, err
	} else if !exists {
		if err := in.createService(p); err != nil {
			return r, err
		}
	}
	if err := tx.Advance(&phase, state.PhaseServiceCreated, "service created"); err != nil {
		return r, err
	}

	// 6. Resolve service SID + apply/verify ACLs.
	serviceSID, err := security.ServiceSIDString(p.ServiceName)
	if err != nil {
		return r, diagnostics.Wrap(diagnostics.ACLFailed("cannot resolve service SID"), err)
	}
	if err := in.applyACLs(p, serviceSID); err != nil {
		return r, err
	}
	if err := tx.Advance(&phase, state.PhaseACLVerified, "acl verified"); err != nil {
		return r, err
	}

	// 7. Persist final password.
	if err := in.writeFinalPassword(p, password); err != nil {
		return r, err
	}

	// 8. Start + verify (unless --no-start).
	serverVersion := ""
	status := state.StatusInstalled
	if o.NoStart {
		status = state.StatusInstalledNotStart
		if err := tx.Advance(&phase, state.PhaseInstalledNotStart, "installed, not started"); err != nil {
			return r, err
		}
	} else {
		if err := tx.Advance(&phase, state.PhaseServiceStarting, "starting service"); err != nil {
			return r, err
		}
		serverVersion, err = in.startAndVerify(ctx, p, password)
		if err != nil {
			return r, err
		}
		if err := tx.Advance(&phase, state.PhaseHealthy, "healthy"); err != nil {
			return r, err
		}
	}

	// 9. Commit state.
	if err := in.commitState(p, verdict.Label, serverVersion, status); err != nil {
		return r, err
	}
	_ = tx.Advance(&phase, state.PhaseCommitted, "committed")

	r.Status = status
	r.PostgresVersion = serverVersion
	r.ConnectionURL = connectionURL(p.Port)
	r.CredentialsHint = credentialsHint(p.Root)
	return r, nil
}

func (in *InstallerImpl) ensureTree(p *plan.Plan) error {
	dirs := []string{
		p.Paths.Root,
		filepath.Dir(p.Paths.Runtime), // <root>\runtime (pgsql subdir is created by rename)
		p.Paths.Data,
		p.Paths.Logs,
		p.Paths.LogsPostgres,
		p.Paths.Secrets,
		p.Paths.State,
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o700); err != nil {
			return diagnostics.Wrap(diagnostics.Internal("cannot create directory %s", d), err)
		}
	}
	// Secrets is admin/SYSTEM only from the start, before any password file.
	if err := security.ApplyDirACL(p.Paths.Secrets, security.RoleSecrets, ""); err != nil {
		return err
	}
	return nil
}

func (in *InstallerImpl) ensureRuntime(ctx context.Context, p *plan.Plan, tx *Transaction, phase *string) error {
	if len(postgres.MissingRuntimeFiles(p.Paths.RuntimeBin)) == 0 {
		if _, err := postgres.Version(ctx, p.Paths.RuntimeBin); err == nil {
			tx.Logger.Info("runtime", "runtime already present, skipping extraction")
			return nil
		}
	}

	staging, err := archive.NewStagingDir(filepath.Join(p.Root, "staging"))
	if err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot create staging directory"), err)
	}
	defer os.RemoveAll(staging)
	if err := security.ApplyDirACL(staging, security.RoleStaging, ""); err != nil {
		return err
	}

	tx.Logger.Info("archive.extract", "extracting archive")
	if err := tx.Advance(phase, state.PhaseRuntimeStaged, "runtime staged"); err != nil {
		return err
	}
	destPgsql := filepath.Join(staging, "pgsql")
	if err := archive.Extract(p.ZipPath, destPgsql); err != nil {
		return err
	}

	tx.Logger.Info("runtime.validate", "validating extracted binaries")
	binDir := filepath.Join(destPgsql, "bin")
	if err := postgres.CheckArchitecture(binDir); err != nil {
		return err
	}
	if _, err := postgres.Version(ctx, binDir); err != nil {
		return err
	}
	if err := tx.Advance(phase, state.PhaseRuntimeReady, "runtime ready"); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(p.Paths.Runtime), 0o700); err != nil {
		return err
	}
	if err := replaceRuntime(destPgsql, p.Paths.Runtime); err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot move runtime into place"), err)
	}
	return nil
}

func (in *InstallerImpl) initDB(ctx context.Context, p *plan.Plan, tx *Transaction, phase *string, password string) error {
	if err := tx.Advance(phase, state.PhaseDataInitializing, "initializing database"); err != nil {
		return err
	}
	pwPath, err := security.WriteSecretFile(p.Paths.Secrets, ".initdb-pw-"+tx.TxID, []byte(password+"\n"))
	if err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot write temporary password file"), err)
	}
	defer os.Remove(pwPath)

	tx.Logger.Info("initdb", "running initdb")
	if err := postgres.InitDB(ctx, p.Paths.RuntimeBin, p.Paths.Data, pwPath); err != nil {
		return err
	}
	if err := tx.Advance(phase, state.PhaseDataInitialized, "database initialized"); err != nil {
		return err
	}
	return nil
}

func (in *InstallerImpl) writeConfig(p *plan.Plan) error {
	if err := postgres.UpdatePostgresConf(filepath.Join(p.Paths.Data, "postgresql.conf"), p.Port, p.Root); err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot write postgresql.conf"), err)
	}
	if err := postgres.WriteHBA(filepath.Join(p.Paths.Data, "pg_hba.conf")); err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot write pg_hba.conf"), err)
	}
	return nil
}

func (in *InstallerImpl) createService(p *plan.Plan) error {
	startType := winservice.StartAutomatic
	if p.Options.ManualStart {
		startType = winservice.StartManual
	}
	spec := winservice.ServiceSpec{
		Name:                p.ServiceName,
		DisplayName:         "PGDeploy PostgreSQL 18 (" + p.ServiceName + ")",
		BinaryPath:          filepath.Join(p.Paths.RuntimeBin, "pg_ctl.exe"),
		Args:                []string{"runservice", "-N", p.ServiceName, "-D", p.Paths.Data, "-w"},
		ServiceStartName:    p.ServiceAccount,
		StartType:           startType,
		Dependencies:        []string{"RpcSs"},
		Description:         "PostgreSQL 18 managed by PGDeploy",
		SidTypeUnrestricted: true,
	}
	return in.SCM.Create(spec)
}

func (in *InstallerImpl) applyACLs(p *plan.Plan, serviceSID string) error {
	apply := []struct {
		path string
		role security.ACLRole
	}{
		{p.Paths.Root, security.RoleRoot},
		{p.Paths.State, security.RoleRoot},
		{filepath.Dir(p.Paths.Runtime), security.RoleRoot},
		{p.Paths.Runtime, security.RoleRuntime},
		{p.Paths.Data, security.RoleData},
		{p.Paths.Logs, security.RoleLogsDir},
		{p.Paths.LogsPostgres, security.RoleLogsPostgres},
		{p.Paths.Secrets, security.RoleSecrets},
	}
	for _, a := range apply {
		if err := security.ApplyDirACL(a.path, a.role, serviceSID); err != nil {
			return err
		}
	}
	for _, a := range apply {
		if err := security.VerifyDirACL(a.path, a.role, serviceSID); err != nil {
			return diagnostics.Wrap(diagnostics.ACLFailed("ACL verification failed"), err)
		}
	}
	return nil
}

func (in *InstallerImpl) writeFinalPassword(p *plan.Plan, password string) error {
	_, err := security.WriteSecretFile(p.Paths.Secrets, "postgres-password.txt", []byte(password+"\n"))
	if err != nil {
		return diagnostics.Wrap(diagnostics.Internal("cannot persist database password"), err)
	}
	return nil
}

func (in *InstallerImpl) startAndVerify(ctx context.Context, p *plan.Plan, password string) (string, error) {
	if err := in.SCM.Start(p.ServiceName); err != nil {
		return "", err
	}
	if err := postgres.WaitReady(ctx, p.Paths.RuntimeBin, p.Port, 60*time.Second); err != nil {
		return "", err
	}
	info, err := postgres.SQLVerify(ctx, p.Paths.RuntimeBin, p.Port, password, p.Paths.Data, "18")
	if err != nil {
		return "", err
	}
	return postgres.FormatVersionNum(info.ServerVersionNum), nil
}

func (in *InstallerImpl) commitState(p *plan.Plan, sourceLabel, serverVersion, status string) error {
	startType := "automatic"
	if p.Options.ManualStart {
		startType = "manual"
	}
	st := &state.InstallState{
		Schema:             state.SchemaVersion,
		InstallerVersion:   version.Version,
		InstanceID:         NewTxID(),
		ServiceName:        p.ServiceName,
		ServiceAccount:     p.ServiceAccount,
		Root:               p.Root,
		Runtime:            p.Paths.Runtime,
		Data:               p.Paths.Data,
		Port:               p.Port,
		PostgresVersion:    serverVersion,
		ZipSHA256:          p.Inspection.SHA256,
		SourceAuthenticity: sourceLabel,
		ServiceStartType:   startType,
		InstalledAt:        time.Now().UTC().Format(time.RFC3339),
		Status:             status,
	}
	return state.Save(installStatePath(p.Paths), st)
}

func (in *InstallerImpl) serviceOwnedByUs(p *plan.Plan) (bool, error) {
	exists, err := in.SCM.Exists(p.ServiceName)
	if err != nil || !exists {
		return true, err // no service, nothing to conflict with
	}
	spec, err := in.SCM.Config(p.ServiceName)
	if err != nil {
		return false, err
	}
	return serviceConfigOwnedBy(spec, &state.InstallState{
		ServiceName: p.ServiceName, Runtime: p.Paths.Runtime, Data: p.Paths.Data,
	}), nil
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func connectionURL(port int) string {
	return fmt.Sprintf("postgresql://postgres@127.0.0.1:%d/postgres", port)
}

func credentialsHint(root string) string {
	return fmt.Sprintf("run `pgdeploy.exe credentials show --root %q` as administrator", root)
}

func volumeName(root string) string {
	clean := filepath.Clean(root)
	if len(clean) >= 2 && clean[1] == ':' {
		return clean[:2] + "\\"
	}
	return clean
}
