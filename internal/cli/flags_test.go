package cli

import (
	"strings"
	"testing"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

func TestInstallFlagsDefaults(t *testing.T) {
	o, err := installFlags(nil)
	if err != nil {
		t.Fatalf("installFlags(nil): %v", err)
	}
	if o.ServiceName != defaultServiceName {
		t.Errorf("ServiceName = %q, want %q", o.ServiceName, defaultServiceName)
	}
	if o.Port != defaultPort {
		t.Errorf("Port = %d, want %d", o.Port, defaultPort)
	}
	if o.ManualStart || o.NoStart || o.JSON || o.Verbose {
		t.Errorf("unexpected default flags: %+v", o)
	}
}

func TestInstallFlagsCustom(t *testing.T) {
	o, err := installFlags([]string{
		"--service", "MyPG18",
		"--port", "55432",
		"--root", `D:\Apps\PostgreSQL`,
		"--zip", "postgresql-18.6-5-windows-x64-binaries.zip",
		"--sha256", strings.Repeat("a", 64),
		"--manual-start", "--no-start", "--json", "--verbose",
	})
	if err != nil {
		t.Fatalf("installFlags: %v", err)
	}
	if o.ServiceName != "MyPG18" || o.Port != 55432 {
		t.Errorf("service/port = %q/%d", o.ServiceName, o.Port)
	}
	if o.RootPath != `D:\Apps\PostgreSQL` {
		t.Errorf("RootPath = %q", o.RootPath)
	}
	if !o.ManualStart || !o.NoStart || !o.JSON || !o.Verbose {
		t.Errorf("bool flags not set: %+v", o)
	}
}

func TestInstallFlagsRejectsBadInput(t *testing.T) {
	cases := [][]string{
		{"--port", "70000"},                   // out of range
		{"--port", "0"},                       // out of range
		{"--service", "1bad"},                 // leading digit
		{"--service", "bad name"},             // space
		{"--sha256", "short"},                 // bad hex length
		{"--sha256", strings.Repeat("g", 64)}, // non-hex
		{"--bogus"},                           // unknown flag
		{"positional"},                        // stray positional
	}
	for _, args := range cases {
		_, err := installFlags(args)
		if err == nil {
			t.Errorf("installFlags(%v) = nil error, want usage error", args)
			continue
		}
		if diagnostics.ExitCode(err) != diagnostics.ExitUsage {
			t.Errorf("installFlags(%v) exit = %d, want %d", args, diagnostics.ExitCode(err), diagnostics.ExitUsage)
		}
	}
}

func TestUninstallFlags(t *testing.T) {
	o, purge, err := uninstallFlags([]string{"--service", "MyPG18", "--purge-data"})
	if err != nil {
		t.Fatalf("uninstallFlags: %v", err)
	}
	if !purge {
		t.Error("--purge-data not captured")
	}
	if o.ServiceName != "MyPG18" {
		t.Errorf("ServiceName = %q", o.ServiceName)
	}

	_, purge, err = uninstallFlags(nil)
	if err != nil {
		t.Fatalf("uninstallFlags(nil): %v", err)
	}
	if !purge {
		t.Error("uninstall must default to complete removal")
	}
}

func TestVerifyFlags(t *testing.T) {
	o, err := verifyFlags([]string{"--sha256", strings.Repeat("f", 64)})
	if err != nil {
		t.Fatalf("verifyFlags: %v", err)
	}
	if o.ExpectedSHA256 != strings.Repeat("f", 64) {
		t.Errorf("ExpectedSHA256 = %q", o.ExpectedSHA256)
	}
}

func TestManagementAndCredentialsFlags(t *testing.T) {
	o, err := managementFlags("status", []string{"--root", `D:\pg`, "--service", "Svc"})
	if err != nil {
		t.Fatalf("managementFlags: %v", err)
	}
	if o.RootPath != `D:\pg` || o.ServiceName != "Svc" {
		t.Errorf("managementFlags = %+v", o)
	}

	c, err := credentialsFlags([]string{"--service", "Svc"})
	if err != nil {
		t.Fatalf("credentialsFlags: %v", err)
	}
	if c.ServiceName != "Svc" {
		t.Errorf("credentialsFlags service = %q", c.ServiceName)
	}
}

func TestIsHex64(t *testing.T) {
	if !isHex64(strings.Repeat("a", 64)) {
		t.Error("lowercase hex rejected")
	}
	if !isHex64(strings.Repeat("A", 64)) {
		t.Error("uppercase hex rejected")
	}
	if isHex64(strings.Repeat("a", 63)) {
		t.Error("63 chars accepted")
	}
	if isHex64(strings.Repeat("g", 64)) {
		t.Error("non-hex accepted")
	}
}

func TestRepairSourceFlags(t *testing.T) {
	hash := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	o, err := managementFlags("repair", []string{"--zip", "archive.zip", "--sha256", hash})
	if err != nil || o.ZipPath != "archive.zip" || o.ExpectedSHA256 != hash {
		t.Fatalf("repair flags: %+v, %v", o, err)
	}
	if _, err := managementFlags("repair", []string{"--sha256", "invalid"}); err == nil {
		t.Fatal("accepted malformed hash")
	}
}

func TestUninstallConfirmationFlags(t *testing.T) {
	o, purge, err := uninstallFlags([]string{"--yes"})
	if err != nil || !o.AssumeYes || !purge {
		t.Fatalf("flags: %+v %v %v", o, purge, err)
	}
	if _, _, err := uninstallFlags([]string{"--purge-data=false"}); err == nil {
		t.Fatal("accepted data-preserving uninstall")
	}
}
