//go:build windows

package cli

import (
	"bytes"
	"strings"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"github.com/pgdeploy/pgdeploy/internal/install"
	"github.com/pgdeploy/pgdeploy/internal/testutil"
)

func newApp() (*App, *fakeInstaller, *testutil.FakeElevator, *bytes.Buffer, *bytes.Buffer) {
	inst := newFakeInstaller()
	el := &testutil.FakeElevator{}
	var out, errb bytes.Buffer
	app := &App{Installer: inst, Elevator: el, Stdin: strings.NewReader(""), Stdout: &out, Stderr: &errb}
	return app, inst, el, &out, &errb
}

func TestRunUnknownCommand(t *testing.T) {
	app, _, _, _, _ := newApp()
	if code := app.Run([]string{"frobnicate"}); code != diagnostics.ExitUsage {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitUsage)
	}
}

func TestRunVersion(t *testing.T) {
	app, _, _, out, _ := newApp()
	if code := app.Run([]string{"version"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if !strings.Contains(out.String(), "pgdeploy") {
		t.Errorf("version output missing product name: %q", out.String())
	}
}

func TestRunHelp(t *testing.T) {
	app, _, _, out, _ := newApp()
	if code := app.Run([]string{"help"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if !strings.Contains(out.String(), "install") {
		t.Errorf("help output missing usage: %q", out.String())
	}
}

func TestRunStatusDoesNotElevate(t *testing.T) {
	app, inst, el, _, _ := newApp()
	inst.Reports["status"] = install.Report{Status: install.StatusNotFound}
	if code := app.Run([]string{"status"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if el.ElevateCalls != 0 {
		t.Fatalf("status should not elevate, but Elevate called %d times", el.ElevateCalls)
	}
	if len(inst.Calls) != 1 || inst.Calls[0] != "status" {
		t.Fatalf("calls = %v", inst.Calls)
	}
}

func TestRunInstallElevatesWhenNotAdmin(t *testing.T) {
	app, inst, el, _, _ := newApp()
	el.Elevated = false
	el.ElevateCode = diagnostics.ExitOK
	if code := app.Run([]string{"install", "--service", "MyPG"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if el.ElevateCalls != 1 {
		t.Fatalf("Elevate calls = %d, want 1", el.ElevateCalls)
	}
	if len(el.ElevateArgs) != 3 || el.ElevateArgs[0] != "install" ||
		el.ElevateArgs[1] != "--service" || el.ElevateArgs[2] != "MyPG" {
		t.Fatalf("ElevateArgs = %v", el.ElevateArgs)
	}
	if len(inst.Calls) != 0 {
		t.Fatalf("installer should not be called in the unelevated parent: %v", inst.Calls)
	}
}

func TestRunInstallRunsWhenAdmin(t *testing.T) {
	app, inst, el, _, _ := newApp()
	el.Elevated = true
	inst.Reports["install"] = install.Report{Status: install.StatusInstalled, Port: 5432}
	if code := app.Run([]string{"install"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if el.ElevateCalls != 0 {
		t.Fatal("already-elevated install should not elevate")
	}
	if len(inst.Calls) != 1 || inst.Calls[0] != "install" {
		t.Fatalf("calls = %v", inst.Calls)
	}
}

func TestRunDoctorNoElevation(t *testing.T) {
	app, inst, el, _, _ := newApp()
	inst.Reports["doctor"] = install.Report{Status: install.StatusHealthy}
	if code := app.Run([]string{"doctor"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if el.ElevateCalls != 0 {
		t.Fatalf("doctor should not elevate, got %d calls", el.ElevateCalls)
	}
	if len(inst.Calls) != 1 || inst.Calls[0] != "doctor" {
		t.Fatalf("calls = %v", inst.Calls)
	}
}

func TestRunCredentialsRequiresShow(t *testing.T) {
	app, _, _, _, _ := newApp()
	if code := app.Run([]string{"credentials"}); code != diagnostics.ExitUsage {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitUsage)
	}
}

func TestRunCredentialsRunsWhenAdmin(t *testing.T) {
	app, inst, el, out, _ := newApp()
	el.Elevated = true
	inst.CredentialsVal = "hunter2"
	if code := app.Run([]string{"credentials", "show", "--service", "MyPG"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if !strings.Contains(out.String(), "hunter2") {
		t.Errorf("credentials output missing password: %q", out.String())
	}
}

func TestRunUninstallYesRemovesEverything(t *testing.T) {
	app, inst, el, _, _ := newApp()
	el.Elevated = true
	inst.Reports["uninstall"] = install.Report{Status: install.StatusUninstalled}
	if code := app.Run([]string{"uninstall", "--yes"}); code != diagnostics.ExitOK {
		t.Fatalf("exit = %d, want %d", code, diagnostics.ExitOK)
	}
	if !inst.LastUninstallPurge {
		t.Error("uninstall must remove everything")
	}
}

func TestRunUninstallPurgeRefusesNonInteractive(t *testing.T) {
	app, _, el, _, _ := newApp()
	el.Elevated = true
	// Stdin is a strings.Reader, not an *os.File, so purge confirmation must fail.
	code := app.Run([]string{"uninstall", "--purge-data"})
	if code == diagnostics.ExitOK {
		t.Fatal("purge without an interactive console should be refused")
	}
}

func TestRunUninstallDefaultRequiresConfirmation(t *testing.T) {
	app, inst, el, _, _ := newApp()
	el.Elevated = true
	if code := app.Run([]string{"uninstall"}); code == diagnostics.ExitOK {
		t.Fatal("accepted unconfirmed uninstall")
	}
	if len(inst.Calls) != 0 {
		t.Fatal("called installer without confirmation")
	}
}
