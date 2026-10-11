//go:build windows

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/plan"
	"github.com/pgdeploy/pgdeploy/internal/postgres"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

// resolveInstance resolves root + service + paths without touching the ZIP.
func (in *InstallerImpl) resolveInstance(o opts.Options) (string, string, state.Paths, error) {
	if err := plan.ValidateServiceName(o.ServiceName); err != nil {
		return "", "", state.Paths{}, diagnostics.Usage("%v", err)
	}
	var root string
	var err error
	if o.RootPath != "" {
		if err := plan.ValidateAbsoluteRoot(o.RootPath); err != nil {
			return "", "", state.Paths{}, diagnostics.Usage("%v", err)
		}
		root, err = plan.CanonicalDir(o.RootPath)
		if err != nil {
			return "", "", state.Paths{}, diagnostics.Wrap(diagnostics.Usage("cannot resolve --root %q", o.RootPath), err)
		}
		if err := plan.ValidateAbsoluteRoot(root); err != nil {
			return "", "", state.Paths{}, diagnostics.Usage("%v", err)
		}
	} else {
		root, err = plan.DefaultRoot(o.ServiceName)
		if err != nil {
			return "", "", state.Paths{}, diagnostics.Wrap(diagnostics.Internal("cannot resolve ProgramData path"), err)
		}
	}
	if reason := plan.DangerReason(root); reason != "" {
		return "", "", state.Paths{}, diagnostics.Usage("unsafe root: %s", reason)
	}
	return root, o.ServiceName, state.DerivePaths(root), nil
}

// loadState returns the install state if present and valid.
func loadState(paths state.Paths) (*state.InstallState, error) {
	st, err := state.Load(installStatePath(paths))
	if err != nil || st == nil {
		return st, err
	}
	if !strings.EqualFold(filepath.Clean(st.Root), paths.Root) || !strings.EqualFold(filepath.Clean(st.Runtime), paths.Runtime) || !strings.EqualFold(filepath.Clean(st.Data), paths.Data) {
		return nil, diagnostics.RecoveryReview("recorded paths do not match the requested instance root")
	}
	return st, nil
}

// Status reports the current instance status.
func (in *InstallerImpl) Status(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	r := Report{Root: root, ServiceName: service}

	st, err := loadState(paths)
	if err != nil {
		return r, err
	}
	if st == nil {
		r.Status = StatusNotFound
		r.Code = diagnostics.CodeRuntimeMissing
		return r, nil
	}
	r.Port = st.Port
	r.DataDir = st.Data
	r.PostgresVersion = st.PostgresVersion

	svcStatus, err := in.SCM.Query(service)
	if err != nil {
		r.Status = st.Status
		r.Warnings = append(r.Warnings, "service state unavailable: "+err.Error())
		return r, nil
	}
	switch svcStatus.State {
	case winserviceStateRunning:
		r.Status = StatusRunning
	case winserviceStateStopped:
		r.Status = StatusStopped
	default:
		r.Status = svcStatus.State
	}
	r.ConnectionURL = connectionURL(st.Port)
	return r, nil
}

// Start starts the service and waits for readiness.
func (in *InstallerImpl) Start(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	st, err := loadState(paths)
	if err != nil {
		return Report{}, err
	}
	if st == nil {
		return Report{Root: root, ServiceName: service, Status: StatusNotFound}, nil
	}
	r := Report{Root: root, ServiceName: service, Port: st.Port, DataDir: st.Data}
	if err := in.SCM.Start(service); err != nil {
		return r, err
	}
	if err := postgres.WaitReady(ctx, paths.RuntimeBin, st.Port, 60*time.Second); err != nil {
		return r, err
	}
	r.Status = StatusRunning
	r.ConnectionURL = connectionURL(st.Port)
	return r, nil
}

// Stop stops the service and waits for it to stop.
func (in *InstallerImpl) Stop(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	_ = paths
	r := Report{Root: root, ServiceName: service}
	if err := in.SCM.Stop(service); err != nil {
		return r, err
	}
	r.Status = StatusStopped
	return r, nil
}

// Restart stops then starts, verifying the connection.
func (in *InstallerImpl) Restart(ctx context.Context, o opts.Options) (Report, error) {
	if _, err := in.Stop(ctx, o); err != nil {
		return Report{}, err
	}
	return in.Start(ctx, o)
}

// Credentials returns the stored postgres password. The caller must already
// have verified administrator elevation.
func (in *InstallerImpl) Credentials(ctx context.Context, o opts.Options) (string, error) {
	_, _, paths, err := in.resolveInstance(o)
	if err != nil {
		return "", err
	}
	pw, err := os.ReadFile(filepath.Join(paths.Secrets, "postgres-password.txt"))
	if err != nil {
		return "", diagnostics.RuntimeMissing("could not read credentials at %s: %v", filepath.Join(paths.Secrets, "postgres-password.txt"), err)
	}
	return trimTrailingNewline(string(pw)), nil
}

func trimTrailingNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}
	return s
}

// Winservice state string aliases.
const (
	winserviceStateRunning = "running"
	winserviceStateStopped = "stopped"
)
