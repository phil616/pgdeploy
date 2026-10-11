package postgres

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// Managed block markers.
const (
	managedBegin = "# BEGIN PGDEPLOY MANAGED"
	managedEnd   = "# END PGDEPLOY MANAGED"
)

// quotePGValue renders a PostgreSQL single-quoted literal, doubling embedded
// single quotes and escaping backslashes. It rejects control characters and
// newlines, which cannot appear safely in a config literal.
func quotePGValue(v string) (string, error) {
	if v == "" {
		return "''", nil
	}
	for _, r := range v {
		if r == '\n' || r == '\r' || unicode.IsControl(r) {
			return "", fmt.Errorf("path contains control characters and cannot be embedded in postgresql.conf")
		}
	}
	v = strings.ReplaceAll(v, `\`, `\\`)
	v = strings.ReplaceAll(v, `'`, `''`)
	return "'" + v + "'", nil
}

// managedBlock renders the managed configuration block. logDirectory must be
// an absolute path using forward slashes.
func managedBlock(port int, logDirectory string) (string, error) {
	logDir, err := quotePGValue(logDirectory)
	if err != nil {
		return "", err
	}
	return strings.Join([]string{
		managedBegin,
		"listen_addresses = '127.0.0.1'",
		fmt.Sprintf("port = %d", port),
		"password_encryption = 'scram-sha-256'",
		"shared_buffers = '32MB'",
		"max_connections = 20",
		"logging_collector = on",
		"log_destination = 'stderr'",
		"log_directory = " + logDir,
		"log_filename = 'postgresql-%Y-%m-%d.log'",
		"log_rotation_age = '1d'",
		"log_rotation_size = '10MB'",
		managedEnd,
	}, "\n") + "\n", nil
}

// UpdatePostgresConf appends (or replaces) the managed block in
// postgresql.conf, preserving everything else in the file. It never rewrites
// user configuration outside the managed block (section 9).
func UpdatePostgresConf(path string, port int, root string) error {
	content, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	logDir := filepath.ToSlash(filepath.Join(root, "logs", "postgres"))
	block, err := managedBlock(port, logDir)
	if err != nil {
		return err
	}

	text := string(content)
	if i := strings.Index(text, managedBegin); i >= 0 {
		if j := strings.Index(text[i:], managedEnd); j >= 0 {
			end := i + j + len(managedEnd) + 1 // consume trailing newline
			if end > len(text) {
				end = len(text)
			}
			text = text[:i] + block + text[end:]
		} else {
			return diagnostics.Internal("managed block is unterminated in %s", path)
		}
	} else {
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		text += "\n" + block
	}

	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		return err
	}
	return nil
}

// hbaContent is the canonical client-authentication file for a loopback-only,
// SCRAM-only instance (section 9).
const hbaContent = `# PostgreSQL Client Authentication Configuration File
# BEGIN PGDEPLOY MANAGED
# Local loopback only; SCRAM-SHA-256 authentication.
# TYPE  DATABASE        USER            ADDRESS                 METHOD
local   all             all                                     scram-sha-256
host    all             all             127.0.0.1/32            scram-sha-256
host    all             all             ::1/128                 scram-sha-256
host    replication     all             127.0.0.1/32            scram-sha-256
host    replication     all             ::1/128                 scram-sha-256
# END PGDEPLOY MANAGED
`

// WriteHBA writes the canonical pg_hba.conf. It is only called for a fresh
// data directory, never over an existing user-managed file.
func WriteHBA(path string) error {
	return os.WriteFile(path, []byte(hbaContent), 0o600)
}
