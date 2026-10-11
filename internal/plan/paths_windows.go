//go:build windows

package plan

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

// DefaultRoot resolves the instance root under the real ProgramData path via
// SHGetKnownFolderPath (never by trusting the %ProgramData% environment
// variable).
func DefaultRoot(serviceName string) (string, error) {
	pd, err := windows.KnownFolderPath(windows.FOLDERID_ProgramData, 0)
	if err != nil {
		return "", err
	}
	return filepath.Join(pd, "PGDeploy", "instances", serviceName), nil
}
