package install

import (
	"os"
	"path/filepath"
	"strings"
)

// removeInstanceFiles is called only after ownership checks and service
// removal. Keep the state directory until all other instance files are gone,
// so an open database/log file cannot destroy the ownership record first.
// RemoveAll removes symlinks themselves, without traversing their targets.
func removeInstanceFiles(root string) error {
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if strings.EqualFold(entry.Name(), "state") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, entry.Name())); err != nil {
			return err
		}
	}
	if err := os.RemoveAll(filepath.Join(root, "state")); err != nil {
		return err
	}
	return os.Remove(root)
}
