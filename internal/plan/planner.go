//go:build windows

package plan

import (
	"os"
	"path/filepath"

	"github.com/pgdeploy/pgdeploy/internal/archive"
	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

// Plan is the fully resolved, validated installation plan.
type Plan struct {
	Options        opts.Options
	ExeDir         string
	ZipPath        string
	Root           string
	ServiceName    string
	ServiceAccount string
	Port           int
	FreeBytes      uint64
	Inspection     *archive.Inspection
	Paths          state.Paths
}

// Resolve validates all inputs, discovers the ZIP, and runs the read-only
// preflight checks. It performs no writes.
func Resolve(o opts.Options) (*Plan, error) {
	if err := ValidateServiceName(o.ServiceName); err != nil {
		return nil, diagnostics.Usage("%v", err)
	}
	if err := ValidatePort(o.Port); err != nil {
		return nil, diagnostics.Usage("%v", err)
	}

	exe, err := os.Executable()
	if err != nil {
		return nil, diagnostics.Wrap(diagnostics.Internal("cannot locate own executable"), err)
	}
	exeDir := filepath.Dir(exe)

	// Resolve the install root.
	var root string
	if o.RootPath != "" {
		if err := ValidateAbsoluteRoot(o.RootPath); err != nil {
			return nil, diagnostics.Usage("%v", err)
		}
		root, err = CanonicalDir(o.RootPath)
		if err != nil {
			return nil, diagnostics.Wrap(diagnostics.Usage("cannot resolve --root %q", o.RootPath), err)
		}
		if err := ValidateAbsoluteRoot(root); err != nil {
			return nil, diagnostics.Usage("%v", err)
		}
	} else {
		root, err = DefaultRoot(o.ServiceName)
		if err != nil {
			return nil, diagnostics.Wrap(diagnostics.Internal("cannot resolve ProgramData path"), err)
		}
	}
	if reason := DangerReason(root); reason != "" {
		return nil, diagnostics.Usage("unsafe install root %q: %s", root, reason)
	}

	// Preflight: OS support, volume, disk space, port.
	if err := CheckOSSupport(); err != nil {
		return nil, err
	}
	freeBytes, err := CheckVolume(root)
	if err != nil {
		return nil, err
	}

	// ZIP discovery + full read-only inspection.
	zipPath, err := archive.SelectZIP(exeDir, o.ZipPath)
	if err != nil {
		return nil, err
	}
	ins, err := archive.Inspect(zipPath)
	if err != nil {
		return nil, err
	}

	return &Plan{
		Options:        o,
		ExeDir:         exeDir,
		ZipPath:        zipPath,
		Root:           root,
		ServiceName:    o.ServiceName,
		ServiceAccount: security.ServiceAccountName(o.ServiceName),
		Port:           o.Port,
		FreeBytes:      freeBytes,
		Inspection:     ins,
		Paths:          state.DerivePaths(root),
	}, nil
}

// RequiredSpace computes the minimum free space: max(2 GiB, zip uncompressed
// + 1 GiB), with an additional 512 MiB headroom for the initial database.
func (p *Plan) RequiredSpace() uint64 {
	const giB = 1024 * 1024 * 1024
	base := p.Inspection.UncompressedSize + giB
	if base < 2*giB {
		base = 2 * giB
	}
	return uint64(base + 512*1024*1024)
}

// HasEnoughSpace reports whether the target volume has sufficient space.
func (p *Plan) HasEnoughSpace() bool { return p.FreeBytes >= p.RequiredSpace() }
