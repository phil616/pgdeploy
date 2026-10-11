// Package install implements the install / repair / uninstall / doctor /
// status commands as a recoverable, journaled transaction. The interfaces
// and report types in this file are cross-platform so the transaction logic
// can be unit-tested with fakes.
package install

import (
	"context"

	"github.com/pgdeploy/pgdeploy/internal/opts"
)

// Report is the structured result of an operation. It never contains secrets.
type Report struct {
	Status           string   `json:"status"`
	Code             string   `json:"code,omitempty"`
	Root             string   `json:"root"`
	ServiceName      string   `json:"service_name"`
	Port             int      `json:"port"`
	DataDir          string   `json:"data_directory"`
	PostgresVersion  string   `json:"postgres_version,omitempty"`
	ConnectionURL    string   `json:"connection_url,omitempty"`
	CredentialsHint  string   `json:"credentials_hint,omitempty"`
	AlreadyInstalled bool     `json:"already_installed,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	Checks           []Check  `json:"checks,omitempty"`
}

// CheckStatus is a doctor check result.
type CheckStatus string

const (
	CheckPass    CheckStatus = "PASS"
	CheckWarn    CheckStatus = "WARN"
	CheckFail    CheckStatus = "FAIL"
	CheckUnknown CheckStatus = "UNKNOWN"
)

// Check is a single doctor diagnostic item.
type Check struct {
	Name   string      `json:"name"`
	Status CheckStatus `json:"status"`
	Detail string      `json:"detail,omitempty"`
}

// Status values used in Report.
const (
	StatusVerified          = "verified"
	StatusUnverified        = "unverified"
	StatusInstalled         = "installed"
	StatusInstalledNotStart = "installed_not_started"
	StatusAlreadyInstalled  = "already_installed"
	StatusHealthy           = "healthy"
	StatusRunning           = "running"
	StatusStopped           = "stopped"
	StatusUninstalled       = "uninstalled"
	StatusNotFound          = "not_found"
)

// Installer is the high-level facade (section 4).
type Installer interface {
	Verify(ctx context.Context, o opts.Options) (Report, error)
	Install(ctx context.Context, o opts.Options) (Report, error)
	Repair(ctx context.Context, o opts.Options) (Report, error)
	Uninstall(ctx context.Context, o opts.Options, purge bool) (Report, error)
	Status(ctx context.Context, o opts.Options) (Report, error)
	Doctor(ctx context.Context, o opts.Options) (Report, error)
	Start(ctx context.Context, o opts.Options) (Report, error)
	Stop(ctx context.Context, o opts.Options) (Report, error)
	Restart(ctx context.Context, o opts.Options) (Report, error)
	Credentials(ctx context.Context, o opts.Options) (string, error)
}
