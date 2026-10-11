//go:build windows

package install

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/plan"
	"github.com/pgdeploy/pgdeploy/internal/state"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
)

// Uninstall removes the service and the entire owned instance root, including
// data, credentials, logs and state. The legacy purge argument is retained for
// API compatibility; both values now perform a complete uninstall.
func (in *InstallerImpl) Uninstall(ctx context.Context, o opts.Options, _ bool) (Report, error) {
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

	if err := plan.CheckNoReparseAncestors(root); err != nil {
		return r, err
	}
	st, err := loadState(paths)
	if err != nil {
		return r, err
	}

	if st == nil {
		if _, err := os.Lstat(root); os.IsNotExist(err) {
			exists, err := in.SCM.Exists(service)
			if err != nil {
				return r, err
			}
			if !exists {
				r.Status = StatusUninstalled
				return r, nil
			}
		}
		return r, diagnostics.RecoveryReview("no install state; refusing to remove unowned files")
	}

	if !strings.EqualFold(st.ServiceName, service) {
		return r, diagnostics.ServiceConflict("requested service does not match install state")
	}

	// Stop + delete the service.
	exists, err := in.SCM.Exists(service)
	if err != nil {
		return r, err
	}
	if exists {
		spec, err := in.SCM.Config(service)
		if err != nil {
			return r, err
		}
		if !serviceConfigOwnedBy(spec, st) {
			return r, diagnostics.ServiceConflict(
				"service %q is not owned by this instance; refusing to delete it", service)
		}
		if err := in.SCM.Stop(service); err != nil {
			return r, err
		}
		if err := in.SCM.Delete(service); err != nil {
			return r, err
		}
	}

	// DeleteService can remain pending while another process holds a handle.
	deadline := time.Now().Add(30 * time.Second)
	for {
		exists, err := in.SCM.Exists(service)
		if err != nil {
			return r, err
		}
		if !exists {
			break
		}
		if time.Now().After(deadline) {
			return r, diagnostics.SCMFailed("service %q is still pending deletion; close service-management windows and retry uninstall", service)
		}
		select {
		case <-ctx.Done():
			return r, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}

	if err := removeInstanceFiles(root); err != nil {
		// Keep an ownership record so a failed removal can be retried.
		if saveErr := state.Save(installStatePath(paths), st); saveErr != nil {
			return r, diagnostics.Internal("uninstall incomplete: %v; cannot retain ownership record: %v", err, saveErr)
		}
		return r, diagnostics.Wrap(diagnostics.Internal("cannot remove instance root %s; uninstall incomplete", root), err)
	}

	// Remove only empty shared containers of the default layout. Never touch
	// a custom root's parents or another instance's files.
	if defaultRoot, err := plan.DefaultRoot(service); err == nil && strings.EqualFold(root, defaultRoot) {
		_ = os.Remove(filepath.Dir(root))
		_ = os.Remove(filepath.Dir(filepath.Dir(root)))
	}
	r.Status = StatusUninstalled
	return r, nil
}
