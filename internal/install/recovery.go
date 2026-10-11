package install

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/security"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

// acquirePassword never invents a new password for an existing cluster.
func acquirePassword(input, secrets string, initialized bool) (string, error) {
	saved, err := security.ReadPasswordFile(filepath.Join(secrets, "postgres-password.txt"))
	if err != nil && !os.IsNotExist(err) {
		return "", err
	}
	if input != "" {
		supplied, e := security.ReadPasswordFile(input)
		if e != nil {
			return "", diagnostics.Usage("%v", e)
		}
		if err == nil && saved != supplied {
			return "", diagnostics.RecoveryReview("supplied password differs from the saved password; installation cannot change a database password")
		}
		return supplied, nil
	}
	if err == nil {
		return saved, nil
	}
	if initialized {
		return "", diagnostics.RecoveryReview("existing database has no saved password; provide its original --password-file")
	}
	return security.GeneratePassword()
}

// replaceRuntime retains the old tree until promotion succeeds. Both trees
// must be on the same volume. A rollback failure preserves the backup path.
func replaceRuntime(staged, target string) error {
	if _, err := os.Stat(target); os.IsNotExist(err) {
		return os.Rename(staged, target)
	} else if err != nil {
		return err
	}
	backup := target + ".backup-" + NewTxID()
	if err := os.Rename(target, backup); err != nil {
		return err
	}
	if err := os.Rename(staged, target); err != nil {
		if rollback := os.Rename(backup, target); rollback != nil {
			return errors.Join(err, fmt.Errorf("runtime backup retained at %s: %w", backup, rollback))
		}
		return err
	}
	return os.RemoveAll(backup)
}

func serviceArgsOwnedBy(args []string, st *state.InstallState) bool {
	if len(args) != 7 {
		return false
	}
	return strings.EqualFold(filepath.Clean(args[0]), filepath.Join(st.Runtime, "bin", "pg_ctl.exe")) &&
		args[1] == "runservice" && args[2] == "-N" && strings.EqualFold(args[3], st.ServiceName) &&
		args[4] == "-D" && strings.EqualFold(filepath.Clean(args[5]), filepath.Clean(st.Data)) && args[6] == "-w"
}
