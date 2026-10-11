// Package plan resolves paths, validates inputs, and runs the pre-install
// preflight checks. The pure string-validation functions in this file are
// cross-platform; volume/reparse/OS checks live in the Windows-tagged files.
package plan

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

// serviceNamePattern matches the legal service-name grammar (section 2.1).
var serviceNamePattern = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_-]{0,63}$`)

// MinPort / MaxPort bound the TCP port.
const (
	MinPort = 1
	MaxPort = 65535
)

// ValidateServiceName checks the service name grammar.
func ValidateServiceName(name string) error {
	if !serviceNamePattern.MatchString(name) {
		return fmt.Errorf("invalid service name %q: must match %s", name, serviceNamePattern.String())
	}
	return nil
}

// ValidatePort checks the TCP port range.
func ValidatePort(port int) error {
	if port < MinPort || port > MaxPort {
		return fmt.Errorf("invalid port %d: must be in [%d,%d]", port, MinPort, MaxPort)
	}
	return nil
}

// ValidateAbsoluteRoot rejects relative paths, UNC paths, and non-clean
// paths. It does not perform volume-level checks (see Preflight).
func ValidateAbsoluteRoot(root string) error {
	if root == "" {
		return fmt.Errorf("root path is empty")
	}
	if !filepath.IsAbs(root) {
		return fmt.Errorf("root path %q must be absolute", root)
	}
	if strings.HasPrefix(root, `\\`) {
		return fmt.Errorf("root path %q is a UNC path, which is not supported", root)
	}
	if root != filepath.Clean(root) {
		return fmt.Errorf("root path %q is not in canonical form (use %s)", root, filepath.Clean(root))
	}
	return nil
}

// VolumeRoot reports whether path is a drive/volume root such as "C:\".
func VolumeRoot(path string) bool {
	clean := filepath.Clean(path)
	return len(clean) == 3 && clean[1] == ':' && clean[2] == '\\'
}

// DangerReason returns a non-empty reason when path is a dangerous install
// location (volume root, Windows system directory, etc.). Empty string means
// the pure checks pass.
func DangerReason(path string) string {
	clean := filepath.Clean(path)
	up := strings.ToUpper(clean)
	if VolumeRoot(clean) {
		return "the volume root is not an allowed install location"
	}
	// Windows system directory and subdirectories.
	if up == `C:\WINDOWS` || strings.HasPrefix(up, `C:\WINDOWS\`) {
		return "the Windows system directory is not an allowed install location"
	}
	return ""
}

// RootKey returns a stable, canonical key for a root path (used for the
// per-root mutex and deduplication).
func RootKey(root string) string {
	return strings.ToUpper(filepath.Clean(root))
}

// CanonicalDir returns the absolute, cleaned form of a directory path.
func CanonicalDir(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	return filepath.Clean(abs), nil
}

// DirExistsAndEmpty reports whether dir exists and contains no entries.
func DirExistsAndEmpty(dir string) (exists, empty bool, err error) {
	entries, err := filepath.Glob(filepath.Join(dir, "*"))
	if err != nil {
		return false, false, err
	}
	if entries == nil {
		return false, true, nil // no match means the dir is missing or empty
	}
	return true, len(entries) == 0, nil
}
