// Package state persists the installer's own records: the committed
// install.json state file, the transactional journal, and the global
// install mutex. It deliberately stores no secrets.
package state

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// SchemaVersion is the current install.json schema.
const SchemaVersion = 1

// Install status values recorded in install.json.
const (
	StatusInstalled         = "installed"
	StatusInstalledNotStart = "installed_not_started"
	StatusNeedsRepair       = "needs_repair"
	StatusPartial           = "partial"
)

// Source authenticity values recorded in install.json.
const (
	SourceSuppliedHash  = "verified_by_supplied_hash"
	SourceEmbeddedAllow = "verified_by_embedded_allowlist"
	SourceUnverified    = "unverified"
)

// InstallState is the committed installation record (install.json). It never
// contains the database password.
type InstallState struct {
	Schema             int    `json:"schema"`
	InstallerVersion   string `json:"installer_version"`
	InstanceID         string `json:"instance_id"`
	ServiceName        string `json:"service_name"`
	ServiceAccount     string `json:"service_account"`
	Root               string `json:"root"`
	Runtime            string `json:"runtime"`
	Data               string `json:"data"`
	Port               int    `json:"port"`
	PostgresVersion    string `json:"postgres_version"`
	ZipSHA256          string `json:"zip_sha256"`
	SourceAuthenticity string `json:"source_authenticity"`
	ServiceStartType   string `json:"service_start_type"`
	InstalledAt        string `json:"installed_at"`
	Status             string `json:"status"`
}

// Paths derived from the state.
type Paths struct {
	Root         string
	Runtime      string
	RuntimeBin   string
	Data         string
	Logs         string
	LogsPostgres string
	Secrets      string
	State        string
}

// DerivePaths computes the standard directory layout from a root path.
func DerivePaths(root string) Paths {
	runtime := filepath.Join(root, "runtime", "pgsql")
	return Paths{
		Root:         root,
		Runtime:      runtime,
		RuntimeBin:   filepath.Join(runtime, "bin"),
		Data:         filepath.Join(root, "data"),
		Logs:         filepath.Join(root, "logs"),
		LogsPostgres: filepath.Join(root, "logs", "postgres"),
		Secrets:      filepath.Join(root, "secrets"),
		State:        filepath.Join(root, "state"),
	}
}

// Load reads and validates an install.json file. A missing file is not an
// error; the returned state is nil.
func Load(path string) (*InstallState, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var st InstallState
	if err := json.Unmarshal(b, &st); err != nil {
		return nil, fmt.Errorf("corrupt state file %s: %w", path, err)
	}
	if st.Schema != SchemaVersion {
		return nil, fmt.Errorf("unsupported state schema %d (expected %d)", st.Schema, SchemaVersion)
	}
	return &st, nil
}

// Save atomically writes the state file via a temporary file and rename on
// the same volume.
func Save(path string, st *InstallState) error {
	if st == nil {
		return errors.New("cannot save nil state")
	}
	if st.Schema == 0 {
		st.Schema = SchemaVersion
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, b)
}

// atomicWrite writes data to a sibling temp file and renames it into place.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName) // no-op after successful rename

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}
