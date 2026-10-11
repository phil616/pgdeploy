// Package cli parses command-line arguments, dispatches commands, and renders
// human-readable or JSON output. Flag parsing and output formatting in this
// file are pure Go and cross-platform testable; command dispatch and UAC
// elevation live in the Windows-tagged files.
package cli

import (
	"flag"
	"io"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/plan"
)

// Defaults from the specification (section 2.1).
const (
	defaultServiceName = "PGDeployPostgreSQL18"
	defaultPort        = 5432
)

func newFlagSet(name string) *flag.FlagSet {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(io.Discard) // we render our own usage and errors
	return fs
}

// parse runs fs.Parse and rejects trailing positional arguments. Unknown
// flags, malformed values, and stray positionals are usage errors.
func parse(fs *flag.FlagSet, args []string) error {
	if err := fs.Parse(args); err != nil {
		return diagnostics.Usage("%v", err)
	}
	if fs.NArg() != 0 {
		return diagnostics.Usage("unexpected argument(s): %v", fs.Args())
	}
	return nil
}

// installFlags parses `install` arguments.
func installFlags(args []string) (opts.Options, error) {
	fs := newFlagSet("install")
	var o opts.Options
	o.ServiceName = defaultServiceName
	o.Port = defaultPort
	fs.StringVar(&o.ZipPath, "zip", "", "path to the PostgreSQL ZIP")
	fs.StringVar(&o.RootPath, "root", "", "absolute install root")
	fs.StringVar(&o.ServiceName, "service", defaultServiceName, "Windows service name")
	fs.IntVar(&o.Port, "port", defaultPort, "TCP port")
	fs.StringVar(&o.ExpectedSHA256, "sha256", "", "expected ZIP SHA-256")
	fs.StringVar(&o.PasswordFile, "password-file", "", "path to a password file (read, not deleted)")
	fs.BoolVar(&o.ManualStart, "manual-start", false, "do not auto-start on boot")
	fs.BoolVar(&o.NoStart, "no-start", false, "install without starting")
	fs.BoolVar(&o.JSON, "json", false, "JSON output")
	fs.BoolVar(&o.Verbose, "verbose", false, "verbose output")
	if err := parse(fs, args); err != nil {
		return o, err
	}
	if err := plan.ValidatePort(o.Port); err != nil {
		return o, diagnostics.Usage("%v", err)
	}
	if err := plan.ValidateServiceName(o.ServiceName); err != nil {
		return o, diagnostics.Usage("%v", err)
	}
	if o.ExpectedSHA256 != "" && !isHex64(o.ExpectedSHA256) {
		return o, diagnostics.Usage("--sha256 must be a 64-character hexadecimal string")
	}
	return o, nil
}

// verifyFlags parses `verify` arguments.
func verifyFlags(args []string) (opts.Options, error) {
	fs := newFlagSet("verify")
	var o opts.Options
	o.ServiceName = defaultServiceName
	o.Port = defaultPort
	fs.StringVar(&o.ZipPath, "zip", "", "path to the PostgreSQL ZIP")
	fs.StringVar(&o.ExpectedSHA256, "sha256", "", "expected ZIP SHA-256")
	fs.BoolVar(&o.JSON, "json", false, "JSON output")
	fs.BoolVar(&o.Verbose, "verbose", false, "verbose output")
	if err := parse(fs, args); err != nil {
		return o, err
	}
	if o.ExpectedSHA256 != "" && !isHex64(o.ExpectedSHA256) {
		return o, diagnostics.Usage("--sha256 must be a 64-character hexadecimal string")
	}
	return o, nil
}

// managementFlags parses the shared flags of status/start/stop/restart/repair/
// doctor: --root, --service, --json, --verbose.
func managementFlags(cmd string, args []string) (opts.Options, error) {
	fs := newFlagSet(cmd)
	var o opts.Options
	o.ServiceName = defaultServiceName
	o.Port = defaultPort
	fs.StringVar(&o.RootPath, "root", "", "absolute install root")
	fs.StringVar(&o.ServiceName, "service", defaultServiceName, "Windows service name")
	fs.BoolVar(&o.JSON, "json", false, "JSON output")
	fs.BoolVar(&o.Verbose, "verbose", false, "verbose output")
	if cmd == "repair" {
		fs.StringVar(&o.ZipPath, "zip", "", "trusted runtime ZIP")
		fs.StringVar(&o.ExpectedSHA256, "sha256", "", "expected ZIP SHA-256")
	}
	if err := parse(fs, args); err != nil {
		return o, err
	}
	if err := plan.ValidateServiceName(o.ServiceName); err != nil {
		return o, diagnostics.Usage("%v", err)
	}
	if o.ExpectedSHA256 != "" && !isHex64(o.ExpectedSHA256) {
		return o, diagnostics.Usage("--sha256 must be a 64-character hexadecimal string")
	}
	return o, nil
}

// uninstallFlags parses `uninstall` arguments, returning the purge flag.
func uninstallFlags(args []string) (opts.Options, bool, error) {
	fs := newFlagSet("uninstall")
	var o opts.Options
	o.ServiceName = defaultServiceName
	o.Port = defaultPort
	var purge bool
	fs.StringVar(&o.RootPath, "root", "", "absolute install root")
	fs.StringVar(&o.ServiceName, "service", defaultServiceName, "Windows service name")
	fs.BoolVar(&o.JSON, "json", false, "JSON output")
	fs.BoolVar(&o.Verbose, "verbose", false, "verbose output")
	fs.BoolVar(&purge, "purge-data", true, "deprecated: uninstall always removes all instance files")
	fs.BoolVar(&o.AssumeYes, "yes", false, "confirm permanent removal without prompting")
	if err := parse(fs, args); err != nil {
		return o, false, err
	}
	if err := plan.ValidateServiceName(o.ServiceName); err != nil {
		return o, false, diagnostics.Usage("%v", err)
	}
	if !purge {
		return o, false, diagnostics.Usage("uninstall always removes all instance files; --purge-data=false is no longer supported")
	}
	return o, true, nil
}

// credentialsFlags parses `credentials show` arguments (--root, --service).
func credentialsFlags(args []string) (opts.Options, error) {
	fs := newFlagSet("credentials show")
	var o opts.Options
	o.ServiceName = defaultServiceName
	o.Port = defaultPort
	fs.StringVar(&o.RootPath, "root", "", "absolute install root")
	fs.StringVar(&o.ServiceName, "service", defaultServiceName, "Windows service name")
	if err := parse(fs, args); err != nil {
		return o, err
	}
	if err := plan.ValidateServiceName(o.ServiceName); err != nil {
		return o, diagnostics.Usage("%v", err)
	}
	return o, nil
}

// isHex64 reports whether s is a 64-character hexadecimal string.
func isHex64(s string) bool {
	if len(s) != 64 {
		return false
	}
	for _, c := range s {
		switch {
		case c >= '0' && c <= '9':
		case c >= 'a' && c <= 'f':
		case c >= 'A' && c <= 'F':
		default:
			return false
		}
	}
	return true
}
