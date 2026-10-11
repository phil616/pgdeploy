//go:build windows

package install

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/archive"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/plan"
	"github.com/pgdeploy/pgdeploy/internal/postgres"
	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/source"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

// Doctor runs a battery of environment/instance checks, reporting PASS / WARN
// / FAIL / UNKNOWN per item. It never writes to the system.
func (in *InstallerImpl) Doctor(ctx context.Context, o opts.Options) (Report, error) {
	root, service, paths, err := in.resolveInstance(o)
	if err != nil {
		return Report{}, err
	}
	r := Report{Root: root, ServiceName: service}
	add := func(name string, st CheckStatus, format string, args ...any) {
		detail := ""
		if format != "" {
			detail = fmt.Sprintf(format, args...)
		}
		r.Checks = append(r.Checks, Check{Name: name, Status: st, Detail: detail})
	}

	elevated := in.Elevator.IsElevated()

	// OS support.
	if err := plan.CheckOSSupport(); err != nil {
		add("os_support", CheckFail, "%s", err.Error())
	} else {
		add("os_support", CheckPass, "supported Windows version")
	}

	// Elevation.
	if elevated {
		add("elevation", CheckPass, "running elevated")
	} else {
		add("elevation", CheckWarn, "not elevated; some checks below will be UNKNOWN")
	}

	// Filesystem.
	if ntfs, err := plan.IsNTFS(root); err != nil {
		add("filesystem", CheckUnknown, "cannot determine: %v", err)
	} else if !ntfs {
		add("filesystem", CheckFail, "volume is not NTFS")
	} else {
		add("filesystem", CheckPass, "NTFS")
	}

	// Disk space.
	if free, err := plan.FreeSpace(root); err != nil {
		add("disk_space", CheckUnknown, "cannot query: %v", err)
	} else {
		add("disk_space", CheckPass, "%d bytes free", free)
	}

	// ZIP discovery + structure + authenticity.
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)
	zipPath, zerr := archive.SelectZIP(exeDir, o.ZipPath)
	if zerr != nil {
		add("zip_present", CheckFail, "%s", zerr.Error())
	} else {
		add("zip_present", CheckPass, "%s", filepath.Base(zipPath))
		if ins, err := archive.Inspect(zipPath); err != nil {
			add("zip_structure", CheckFail, "%s", err.Error())
		} else {
			add("zip_structure", CheckPass, "%d entries, %d bytes", ins.EntryCount, ins.UncompressedSize)
			verdict := source.Verify(filepath.Base(zipPath), ins.SHA256, o.ExpectedSHA256)
			if verdict.Verified {
				add("zip_authenticity", CheckPass, "%s", verdict.Label)
			} else {
				add("zip_authenticity", CheckWarn, "%s", verdict.Reason)
			}
		}
	}

	// Runtime.
	if missing := postgres.MissingRuntimeFiles(paths.RuntimeBin); len(missing) == 0 {
		if v, err := postgres.Version(ctx, paths.RuntimeBin); err != nil {
			add("runtime", CheckFail, "%s", err.Error())
		} else {
			add("runtime", CheckPass, "PostgreSQL %s", v)
		}
	} else {
		add("runtime", CheckFail, "missing binaries: %v", missing)
	}

	// State.
	st, err := loadState(paths)
	if err != nil {
		add("state", CheckWarn, "unreadable: %v", err)
	} else if st == nil {
		add("state", CheckWarn, "no install state (not installed here?)")
	} else {
		add("state", CheckPass, "%s (v%s)", st.Status, st.PostgresVersion)
	}

	statePort := 5432
	if st != nil && st.Port != 0 {
		statePort = st.Port
	}

	// Data directory.
	if _, err := os.Stat(filepath.Join(paths.Data, "PG_VERSION")); err == nil {
		add("data_directory", CheckPass, "initialized at %s", paths.Data)
	} else {
		add("data_directory", CheckWarn, "not initialized")
	}

	// Service.
	if exists, err := in.SCM.Exists(service); err != nil {
		add("service", CheckUnknown, "cannot query: %v", err)
	} else if !exists {
		add("service", CheckWarn, "service %q not registered", service)
	} else {
		spec, err := in.SCM.Config(service)
		if err != nil {
			add("service", CheckUnknown, "cannot read config: %v", err)
		} else {
			if st != nil && !serviceConfigOwnedBy(spec, st) {
				add("service", CheckFail, "ImagePath does not point at this instance")
			} else {
				add("service", CheckPass, "account=%s", spec.ServiceStartName)
			}
			expectedAccount := security.ServiceAccountName(service)
			if spec.ServiceStartName != expectedAccount {
				add("service_identity", CheckFail, "runs as %s, expected %s", spec.ServiceStartName, expectedAccount)
			} else {
				add("service_identity", CheckPass, "%s", spec.ServiceStartName)
			}
			if spec.StartType == winservice.StartAutomatic {
				add("service_autostart", CheckPass, "automatic")
			} else {
				add("service_autostart", CheckWarn, "manual start")
			}
		}
		if svc, err := in.SCM.Query(service); err == nil {
			add("service_state", CheckPass, "%s", svc.State)
			if svc.State == winservice.StateRunning {
				if err := postgres.WaitReady(ctx, paths.RuntimeBin, statePort, 5*time.Second); err != nil {
					add("connectivity", CheckFail, "%s", err.Error())
				} else {
					add("connectivity", CheckPass, "accepting connections")
				}
			}
		}
	}

	// ACLs (only when elevated and state exists).
	if elevated && st != nil {
		sid, err := security.ServiceSIDString(service)
		if err != nil {
			add("acl", CheckUnknown, "cannot resolve service SID: %v", err)
		} else if err := security.VerifyDirACL(paths.Secrets, security.RoleSecrets, sid); err != nil {
			add("acl", CheckFail, "%s", err.Error())
		} else {
			add("acl", CheckPass, "secrets ACL protected")
		}
	} else {
		add("acl", CheckUnknown, "requires administrator elevation")
	}

	return r, nil
}
