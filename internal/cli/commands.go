//go:build windows

package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/install"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/version"
	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

// App wires the CLI to its dependencies so it can be exercised with fakes.
type App struct {
	Installer install.Installer
	Elevator  winservice.Elevator
	Stdin     io.Reader
	Stdout    io.Writer
	Stderr    io.Writer
}

// Run dispatches a command from args (excluding argv[0]) and returns the
// process exit code.
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		a.usage(a.Stderr)
		return diagnostics.ExitUsage
	}
	switch args[0] {
	case "help", "-h", "--help", "-help", "/?":
		a.usage(a.Stdout)
		return diagnostics.ExitOK
	case "version", "--version":
		a.printVersion()
		return diagnostics.ExitOK
	case "credentials":
		return a.runCredentials(args)
	case "uninstall":
		return a.runUninstall(args)
	case "install", "verify", "status", "start", "stop", "restart", "repair", "doctor":
		return a.runStandard(args[0], args)
	default:
		fmt.Fprintf(a.Stderr, "unknown command %q\n\n", args[0])
		a.usage(a.Stderr)
		return diagnostics.ExitUsage
	}
}

func (a *App) runStandard(cmd string, args []string) int {
	var (
		o   opts.Options
		err error
	)
	switch cmd {
	case "install":
		o, err = installFlags(args[1:])
	case "verify":
		o, err = verifyFlags(args[1:])
	default:
		o, err = managementFlags(cmd, args[1:])
	}
	if err != nil {
		return a.fail(err, o.JSON)
	}
	if needsAdmin(cmd) {
		if code, handled := a.maybeElevate(args, o); handled {
			return code
		}
	}

	ctx := context.Background()
	var r install.Report
	switch cmd {
	case "install":
		r, err = a.Installer.Install(ctx, o)
	case "verify":
		r, err = a.Installer.Verify(ctx, o)
	case "status":
		r, err = a.Installer.Status(ctx, o)
	case "start":
		r, err = a.Installer.Start(ctx, o)
	case "stop":
		r, err = a.Installer.Stop(ctx, o)
	case "restart":
		r, err = a.Installer.Restart(ctx, o)
	case "repair":
		r, err = a.Installer.Repair(ctx, o)
	case "doctor":
		r, err = a.Installer.Doctor(ctx, o)
	}
	if err != nil {
		return a.fail(err, o.JSON)
	}
	printReport(a.Stdout, r, o.JSON)
	return diagnostics.ExitOK
}

func (a *App) runUninstall(args []string) int {
	o, purge, err := uninstallFlags(args[1:])
	if err != nil {
		return a.fail(err, o.JSON)
	}
	if code, handled := a.maybeElevate(args, o); handled {
		return code
	}
	if !o.AssumeYes {
		if err := a.confirmPurge(o.ServiceName); err != nil {
			return a.fail(err, o.JSON)
		}
	}
	r, err := a.Installer.Uninstall(context.Background(), o, purge)
	if err != nil {
		return a.fail(err, o.JSON)
	}
	printReport(a.Stdout, r, o.JSON)
	return diagnostics.ExitOK
}

func (a *App) runCredentials(args []string) int {
	if len(args) < 2 || args[1] != "show" {
		fmt.Fprintf(a.Stderr, "usage: pgdeploy.exe credentials show [--root <path>] [--service <name>]\n")
		return diagnostics.ExitUsage
	}
	o, err := credentialsFlags(args[2:])
	if err != nil {
		return a.fail(err, false)
	}
	if code, handled := a.maybeElevate(args, o); handled {
		return code
	}
	pw, err := a.Installer.Credentials(context.Background(), o)
	if err != nil {
		return a.fail(err, false)
	}
	fmt.Fprintln(a.Stdout, pw)
	return diagnostics.ExitOK
}

// maybeElevate self-elevates a command that requires administrator rights. It
// returns (code, true) when the parent should exit with the elevated child's
// exit code, and (0, false) when the current process is already elevated.
func (a *App) maybeElevate(args []string, o opts.Options) (int, bool) {
	if a.Elevator.IsElevated() {
		return 0, false
	}
	code, err := a.Elevator.Elevate(args)
	if err != nil {
		a.fail(err, o.JSON)
		return diagnostics.ExitCode(err), true
	}
	return code, true
}

func (a *App) fail(err error, jsonOut bool) int {
	printError(a.Stderr, err, jsonOut)
	return diagnostics.ExitCode(err)
}

func (a *App) printVersion() {
	fmt.Fprintf(a.Stdout, "pgdeploy %s (commit %s, built %s)\n",
		version.Version, version.GitCommit, version.BuildTime)
}

// confirmPurge enforces the interactive secondary authorization for
// complete uninstall: an interactive console must type the full
// service name.
func (a *App) confirmPurge(serviceName string) error {
	f, ok := a.Stdin.(*os.File)
	if !ok || !isTerminal(f) {
		return diagnostics.Usage(
			"uninstall permanently removes the entire instance; use an interactive console or --yes to confirm")
	}
	fmt.Fprintf(a.Stdout, "UNINSTALL will permanently delete service %q and its entire installation directory, including all databases, passwords, logs and state.\n", serviceName)
	fmt.Fprintf(a.Stdout, "Type the full service name (%s) to confirm: ", serviceName)
	var line string
	if _, err := fmt.Fscanln(a.Stdin, &line); err != nil {
		return diagnostics.Usage("could not read confirmation: %v", err)
	}
	if line != serviceName {
		return diagnostics.Usage("confirmation did not match the service name; purge cancelled")
	}
	return nil
}

func needsAdmin(cmd string) bool {
	switch cmd {
	case "install", "start", "stop", "restart", "repair", "uninstall", "credentials":
		return true
	default:
		return false
	}
}

func (a *App) usage(w io.Writer) {
	fmt.Fprint(w, `PGDeploy — install and manage PostgreSQL 18 on Windows.

Usage:
  pgdeploy.exe install [options]            Install PostgreSQL 18 and start it
  pgdeploy.exe verify [options]             Read-only archive/source verification
  pgdeploy.exe status [options]             Show instance + service status
  pgdeploy.exe start [options]              Start the managed service
  pgdeploy.exe stop [options]               Stop the managed service
  pgdeploy.exe restart [options]            Stop then start, with a connection check
  pgdeploy.exe repair [options]             Repair ACLs/service config/runtime files
  pgdeploy.exe doctor [options]             Run diagnostics (PASS/WARN/FAIL/UNKNOWN)
  pgdeploy.exe credentials show [options]   Show the stored postgres password
  pgdeploy.exe uninstall [options]          Permanently remove service and entire instance
  pgdeploy.exe version                      Show version and build metadata
  pgdeploy.exe help                         Show this help

Install options:
  --zip <path>          PostgreSQL ZIP (default: auto-discover next to the EXE)
  --root <path>         Absolute install root (default: under ProgramData)
  --service <name>      Service name (default: PGDeployPostgreSQL18)
  --port <1..65535>     TCP port (default: 5432)
  --sha256 <64-hex>     Expected ZIP SHA-256 from an independent channel
  --password-file <p>   Read a supplied password file (never deleted)
  --manual-start        Do not auto-start the service on boot
  --no-start            Install without starting or SQL verification
  --json                JSON output (never contains the password)
  --verbose             Verbose diagnostic output

Management options (status/start/stop/restart/repair/doctor):
  --root <path>         Absolute install root
  --service <name>      Service name
  --json                JSON output
  --verbose             Verbose output

Repair options:
  --zip <path>         Trusted runtime ZIP to restore missing files
  --sha256 <64-hex>    Expected ZIP SHA-256

Uninstall options:
  --yes                 Confirm complete removal without prompting
  --purge-data          Deprecated; complete removal is now the default
`)
}
