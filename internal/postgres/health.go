package postgres

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// WaitReady repeatedly runs pg_isready until the server accepts connections
// or the timeout elapses.
func WaitReady(ctx context.Context, binDir string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	exe := ExePath(binDir, "pg_isready.exe")
	for {
		cctx, cancel := context.WithTimeout(ctx, 3*time.Second)
		res := run(cctx, binDir, nil, exe, "-h", "127.0.0.1", "-p", strconv.Itoa(port), "-t", "3")
		cancel()
		if res.Err == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return diagnostics.HealthFailed(
				"server did not become ready on 127.0.0.1:%d within %s (last: %s)",
				port, timeout, strings.TrimSpace(res.Stderr))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(time.Second):
		}
	}
}

// SQLInfo is the result of the real SCRAM login verification.
type SQLInfo struct {
	DataDirectory    string
	Port             string
	ListenAddresses  string
	ServerVersionNum string
}

// SQLVerify performs a real SCRAM login against the target instance and
// checks that the connected server is exactly the instance that was planned
// (data directory, port, major version). The password is delivered only via
// the PGPASSWORD environment variable, never in argv (section 11).
func SQLVerify(ctx context.Context, binDir string, port int, password, expectedDataDir, expectedVersion string) (SQLInfo, error) {
	exe := ExePath(binDir, "psql.exe")
	sql := "SELECT current_setting('data_directory'), current_setting('port'), current_setting('listen_addresses'), current_setting('server_version_num');"
	cctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	res := run(cctx, binDir, []string{"PGPASSWORD=" + password},
		exe, "-X", "-w", "-v", "ON_ERROR_STOP=1",
		"-h", "127.0.0.1", "-p", strconv.Itoa(port),
		"-U", "postgres", "-d", "postgres", "-At", "-c", sql,
	)
	if res.Err != nil {
		return SQLInfo{}, diagnostics.HealthFailed(
			"SCRAM login failed on 127.0.0.1:%d (exit %d): %s",
			port, res.ExitCode, strings.TrimSpace(res.Stderr))
	}

	fields := strings.Split(strings.TrimSpace(res.Stdout), "|")
	if len(fields) < 4 {
		return SQLInfo{}, diagnostics.HealthFailed("unexpected SQL verification output: %q", res.Stdout)
	}
	info := SQLInfo{
		DataDirectory:    fields[0],
		Port:             fields[1],
		ListenAddresses:  fields[2],
		ServerVersionNum: fields[3],
	}

	if normalizePath(info.DataDirectory) != normalizePath(expectedDataDir) {
		return info, diagnostics.HealthFailed(
			"connected to the wrong data directory: server=%s expected=%s", info.DataDirectory, expectedDataDir)
	}
	if info.Port != strconv.Itoa(port) {
		return info, diagnostics.HealthFailed("server reported port %s, expected %d", info.Port, port)
	}
	if !strings.HasPrefix(info.ServerVersionNum, expectedVersion) {
		return info, diagnostics.HealthFailed("server major version is %s, expected %s.x", info.ServerVersionNum, expectedVersion)
	}
	if info.DataDirectory == "" {
		return info, diagnostics.HealthFailed("server returned an empty data_directory")
	}
	return info, nil
}

// normalizePath canonicalizes a data-directory path for comparison.
func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	p = strings.Trim(p, `"`)
	p = filepath.ToSlash(p)
	p = strings.TrimRight(p, "/")
	return strings.ToLower(p)
}

// FormatVersionNum turns a server_version_num string like "180006" into a
// dotted "18.6" for display. Unchanged if it does not parse.
func FormatVersionNum(s string) string {
	n, err := strconv.Atoi(s)
	if err != nil || n < 100000 {
		return s
	}
	return fmt.Sprintf("%d.%d", n/10000, n%10000)
}
