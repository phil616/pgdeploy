//go:build windows

package cli

import (
	"context"

	"github.com/pgdeploy/pgdeploy/internal/install"
	"github.com/pgdeploy/pgdeploy/internal/opts"
)

// fakeInstaller records method calls and returns canned reports/errors. It
// lets CLI dispatch and UAC-gating logic be exercised without touching the
// filesystem or the SCM.
type fakeInstaller struct {
	Calls              []string
	Reports            map[string]install.Report
	Err                map[string]error
	CredentialsVal     string
	CredentialsErr     error
	LastUninstallPurge bool
}

func newFakeInstaller() *fakeInstaller {
	return &fakeInstaller{Reports: map[string]install.Report{}, Err: map[string]error{}}
}

func (f *fakeInstaller) Verify(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "verify")
	return f.Reports["verify"], f.Err["verify"]
}
func (f *fakeInstaller) Install(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "install")
	return f.Reports["install"], f.Err["install"]
}
func (f *fakeInstaller) Repair(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "repair")
	return f.Reports["repair"], f.Err["repair"]
}
func (f *fakeInstaller) Uninstall(ctx context.Context, o opts.Options, purge bool) (install.Report, error) {
	f.Calls = append(f.Calls, "uninstall")
	f.LastUninstallPurge = purge
	return f.Reports["uninstall"], f.Err["uninstall"]
}
func (f *fakeInstaller) Status(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "status")
	return f.Reports["status"], f.Err["status"]
}
func (f *fakeInstaller) Doctor(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "doctor")
	return f.Reports["doctor"], f.Err["doctor"]
}
func (f *fakeInstaller) Start(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "start")
	return f.Reports["start"], f.Err["start"]
}
func (f *fakeInstaller) Stop(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "stop")
	return f.Reports["stop"], f.Err["stop"]
}
func (f *fakeInstaller) Restart(ctx context.Context, o opts.Options) (install.Report, error) {
	f.Calls = append(f.Calls, "restart")
	return f.Reports["restart"], f.Err["restart"]
}
func (f *fakeInstaller) Credentials(ctx context.Context, o opts.Options) (string, error) {
	f.Calls = append(f.Calls, "credentials")
	return f.CredentialsVal, f.CredentialsErr
}
