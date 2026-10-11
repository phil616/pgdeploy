//go:build windows && integration

package tests

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/install"
	"github.com/pgdeploy/pgdeploy/internal/opts"
	"github.com/pgdeploy/pgdeploy/internal/source"
)

// TestInstallStatusDoctorUninstall drives a full install against a real ZIP
// and the real SCM, then verifies status/doctor and uninstalls. It requires:
//
//	PGDEPLOY_INTEGRATION=1
//	PGDEPLOY_TEST_SHA256=<64-hex>   (or a populated source.Allowlist)
//	PGDEPLOY_TEST_ZIP=<path>        (optional; defaults to the ZIP next to the binary)
//	Administrator elevation
func TestInstallStatusDoctorUninstall(t *testing.T) {
	if os.Getenv("PGDEPLOY_INTEGRATION") != "1" {
		t.Skip("set PGDEPLOY_INTEGRATION=1 to run integration tests")
	}
	if os.Getenv("PGDEPLOY_TEST_SHA256") == "" && len(source.Allowlist) == 0 {
		t.Skip("set PGDEPLOY_TEST_SHA256 (or populate source.Allowlist) to run the install integration test")
	}

	zipPath := os.Getenv("PGDEPLOY_TEST_ZIP")
	if zipPath == "" {
		exe, _ := os.Executable()
		zipPath = filepath.Join(filepath.Dir(exe), "postgresql-18.6-5-windows-x64-binaries.zip")
	}
	if _, err := os.Stat(zipPath); err != nil {
		t.Fatalf("no archive at %s: %v (set PGDEPLOY_TEST_ZIP)", zipPath, err)
	}

	in := install.New()
	service := "PGDeploy18IT"
	root := filepath.Join(os.TempDir(), "pgdeploy-integration-"+time.Now().Format("20060102-150405.000"))
	o := opts.Options{
		ZipPath:        zipPath,
		RootPath:       root,
		ServiceName:    service,
		Port:           55999,
		ExpectedSHA256: os.Getenv("PGDEPLOY_TEST_SHA256"),
	}
	ctx := context.Background()

	rep, err := in.Install(ctx, o)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if rep.Status != install.StatusInstalled {
		t.Fatalf("install status = %q, want %q", rep.Status, install.StatusInstalled)
	}
	defer func() {
		if _, err := in.Uninstall(ctx, o, true); err != nil {
			t.Errorf("uninstall: %v", err)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("instance directory remains after uninstall: %v", err)
		}
	}()

	if rep, err := in.Install(ctx, o); err != nil || !rep.AlreadyInstalled {
		t.Fatalf("repeat install while running: %+v, %v", rep, err)
	}

	st, err := in.Status(ctx, o)
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Status != install.StatusRunning {
		t.Fatalf("status = %q, want %q", st.Status, install.StatusRunning)
	}

	dr, err := in.Doctor(ctx, o)
	if err != nil {
		t.Fatalf("doctor: %v", err)
	}
	if len(dr.Checks) == 0 {
		t.Fatal("doctor returned no checks")
	}
	for _, check := range dr.Checks {
		if check.Status == install.CheckFail {
			t.Errorf("doctor %s: %s", check.Name, check.Detail)
		}
	}

	if _, err := in.Stop(ctx, o); err != nil {
		t.Fatalf("stop: %v", err)
	}
	if _, err := in.Start(ctx, o); err != nil {
		t.Fatalf("start: %v", err)
	}
}
