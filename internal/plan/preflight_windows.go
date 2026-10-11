//go:build windows

package plan

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"golang.org/x/sys/windows"
)

// CheckOSSupport validates the Windows edition/build against the supported
// matrix: Windows 11 x64 and Windows Server 2022/2025. Windows 10 is
// diagnosed and refused (section 1.1).
func CheckOSSupport() error {
	if runtime.GOARCH != "amd64" {
		return diagnostics.OSUnsupported("only the x64 architecture is supported (got %s)", runtime.GOARCH)
	}
	v := windows.RtlGetVersion()
	build := v.BuildNumber
	productType := v.ProductType // 1 = workstation, 3 = server
	switch {
	case productType == 1 && build >= 22000:
		return nil
	case productType == 3 && build >= 20348:
		return nil
	case productType == 1 && build >= 10240:
		return diagnostics.OSUnsupported(
			"Windows 10 (build %d) is outside the supported matrix; refusing automatic install. Use a supported Windows 11 or Server 2022/2025 host.", build)
	default:
		return diagnostics.OSUnsupported(
			"unsupported Windows build %d (product type %d); only Windows 11 and Server 2022/2025 x64 are supported", build, productType)
	}
}

// volumeRoot returns "C:\" style root for an absolute path.
func volumeRoot(path string) string {
	clean := filepath.Clean(path)
	if len(clean) >= 2 && clean[1] == ':' {
		return clean[:3]
	}
	return clean
}

// FreeSpace returns available bytes on the volume holding path.
func FreeSpace(path string) (uint64, error) {
	root := volumeRoot(path)
	rootp, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return 0, err
	}
	var available, total, free uint64
	if err := windows.GetDiskFreeSpaceEx(rootp, &available, &total, &free); err != nil {
		return 0, err
	}
	return available, nil
}

// IsNTFS reports whether the volume holding path is NTFS.
func IsNTFS(path string) (bool, error) {
	root := volumeRoot(path)
	rootp, err := windows.UTF16PtrFromString(root)
	if err != nil {
		return false, err
	}
	var (
		volName [64]uint16
		serial  uint32
		maxComp uint32
		flags   uint32
		fsName  [64]uint16
	)
	err = windows.GetVolumeInformation(rootp, &volName[0], uint32(len(volName)), &serial, &maxComp, &flags, &fsName[0], uint32(len(fsName)))
	if err != nil {
		return false, err
	}
	fs := windows.UTF16ToString(fsName[:])
	return strings.EqualFold(fs, "NTFS"), nil
}

// CheckVolume validates the target volume is NTFS and free of reparse-point
// ancestors, and reports remaining space.
func CheckVolume(root string) (freeBytes uint64, err error) {
	if !filepath.IsAbs(root) {
		return 0, diagnostics.FSUnsupported("root %q is not absolute", root)
	}
	if ntfs, err := IsNTFS(root); err != nil {
		return 0, diagnostics.Wrap(diagnostics.FSUnsupported("cannot determine filesystem of %s", volumeRoot(root)), err)
	} else if !ntfs {
		return 0, diagnostics.FSUnsupported(
			"volume %s is not NTFS; PostgreSQL requires a local NTFS volume that supports Windows ACLs", volumeRoot(root))
	}
	if err := CheckNoReparseAncestors(root); err != nil {
		return 0, err
	}
	freeBytes, err = FreeSpace(root)
	if err != nil {
		return 0, diagnostics.Wrap(diagnostics.FSUnsupported("cannot query free space on %s", volumeRoot(root)), err)
	}
	return freeBytes, nil
}

// CheckNoReparseAncestors walks from the nearest existing ancestor of root up
// to the volume root, rejecting any reparse point (junction/symlink/mount
// point) in the path (section 6).
func CheckNoReparseAncestors(root string) error {
	cur := filepath.Clean(root)
	for {
		if _, err := os.Stat(cur); err == nil {
			break
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	for {
		attrs, err := windows.GetFileAttributes(windows.StringToUTF16Ptr(cur))
		if err == nil && attrs&windows.FILE_ATTRIBUTE_REPARSE_POINT != 0 {
			return diagnostics.FSUnsupported("path component %q is a reparse point; refusing to install through it", cur)
		}
		parent := filepath.Dir(cur)
		if parent == cur || len(cur) <= 3 {
			break
		}
		cur = parent
	}
	return nil
}

// CheckPort verifies the loopback port is not already bound. This is a
// pre-check with an inherent race window; the final proof is the SQL
// verification after service start (section 6).
func CheckPort(port int) error {
	addr := fmt.Sprintf("127.0.0.1:%d", port)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return diagnostics.PortOccupied("TCP port %d is already in use on 127.0.0.1", port)
	}
	ln.Close()
	return nil
}
