package postgres

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// InitDB runs the native initdb.exe against an empty data directory using the
// PostgreSQL 18 builtin locale and SCRAM authentication. The superuser
// password is delivered via a protected --pwfile, never on the command line.
func InitDB(ctx context.Context, binDir, dataDir, passwordFile string) error {
	initdb := ExePath(binDir, "initdb.exe")
	args := []string{
		initdb,
		"-D", dataDir,
		"-U", "postgres",
		"-E", "UTF8",
		"-A", "scram-sha-256",
		"--locale-provider=builtin",
		"--builtin-locale=C.UTF-8",
		"--pwfile=" + passwordFile,
		"--no-instructions",
	}

	cctx, cancel := context.WithTimeout(ctx, InitDBTimeout)
	defer cancel()

	res := run(cctx, binDir, nil, args...)
	if res.Err != nil {
		return diagnostics.InitDBFailed(
			"initdb failed (exit %d): %s", res.ExitCode, res.Stderr)
	}
	if err := ValidateDataDirectory(dataDir); err != nil {
		return err
	}
	return nil
}

// verifyInitDBOutput confirms the essential files produced by a successful
// initdb (section 8.2).
func ValidateDataDirectory(dataDir string) error {
	v, err := os.ReadFile(filepath.Join(dataDir, "PG_VERSION"))
	if err != nil {
		return diagnostics.InitDBFailed("initdb completed but PG_VERSION is missing: %v", err)
	}
	if strings.TrimSpace(string(v)) != "18" {
		return diagnostics.InitDBFailed("initdb produced PG_VERSION %q, expected 18", strings.TrimSpace(string(v)))
	}
	for _, f := range []string{
		filepath.Join("global", "pg_control"),
		"pg_hba.conf",
		"postgresql.conf",
	} {
		if _, err := os.Stat(filepath.Join(dataDir, filepath.FromSlash(f))); err != nil {
			return diagnostics.InitDBFailed("initdb completed but %s is missing", f)
		}
	}
	return nil
}
