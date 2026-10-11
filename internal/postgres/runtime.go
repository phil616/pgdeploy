// Package postgres drives the native PostgreSQL executables directly via
// os/exec (never through a shell), validates the runtime tree, initializes
// the database, writes configuration, and performs health checks.
package postgres

import (
	"bytes"
	"context"
	"debug/pe"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// maxStreamBytes caps how much stdout/stderr we retain from any tool
// invocation, to prevent pathological output from exhausting memory.
const maxStreamBytes = 2 * 1024 * 1024

// DefaultInitDBTimeout is the default initdb wall-clock timeout.
const DefaultInitDBTimeout = 5 * time.Minute

// InitDBTimeout is configurable for tests.
var InitDBTimeout = DefaultInitDBTimeout

// coreExecutables are the five EXEs the archive must provide.
var coreExecutables = []string{
	"postgres.exe", "initdb.exe", "pg_ctl.exe", "pg_isready.exe", "psql.exe",
}

// ExePath returns the full path to a binary in binDir.
func ExePath(binDir, name string) string {
	return filepath.Join(binDir, name)
}

// RunResult captures a completed native tool invocation.
type RunResult struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

// combinedStderr is a bounded writer that records output up to a cap.
type boundedBuffer struct {
	buf bytes.Buffer
	n   int
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	room := maxStreamBytes - b.n
	if room > 0 {
		if len(p) > room {
			b.buf.Write(p[:room])
			b.n += room
		} else {
			b.buf.Write(p)
			b.n += len(p)
		}
	}
	return len(p), nil
}

func (b *boundedBuffer) String() string {
	if b.n >= maxStreamBytes {
		return b.buf.String() + "\n[output truncated]"
	}
	return b.buf.String()
}

// sanitizedEnv removes PG* variables inherited from the parent so a polluted
// caller environment cannot change tool behavior.
func sanitizedEnv(extra ...string) []string {
	var out []string
	for _, kv := range os.Environ() {
		key := kv
		if i := strings.IndexByte(kv, '='); i >= 0 {
			key = kv[:i]
		}
		if strings.HasPrefix(strings.ToUpper(key), "PG") {
			continue
		}
		out = append(out, kv)
	}
	return append(out, extra...)
}

// run executes a native tool with a clean environment. dir is the working
// directory (the bin dir), and env are extra variables to append.
func run(ctx context.Context, binDir string, env []string, argv ...string) RunResult {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = binDir
	cmd.Env = sanitizedEnv(env...)
	var stdout, stderr boundedBuffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()

	res := RunResult{Stdout: stdout.String(), Stderr: stderr.String()}
	if err != nil {
		res.Err = err
		if ee, ok := err.(*exec.ExitError); ok {
			res.ExitCode = ee.ExitCode()
		} else {
			res.ExitCode = -1
		}
	}
	return res
}

// Version runs each core binary's --version and verifies the major version
// is 18 and all three agree (section 5.3, layer 3).
func Version(ctx context.Context, binDir string) (string, error) {
	type probe struct{ name, version string }
	var results []probe
	for _, name := range []string{"postgres.exe", "initdb.exe", "pg_ctl.exe"} {
		exe := ExePath(binDir, name)
		res := run(ctx, binDir, nil, exe, "--version")
		if res.Err != nil {
			return "", diagnostics.Wrap(diagnostics.RuntimeMissing(
				"failed to run %s --version (exit %d): %s", name, res.ExitCode, res.Stderr), res.Err)
		}
		version, err := parsePostgresVersion(res.Stdout)
		if err != nil {
			return "", diagnostics.RuntimeMissing("could not parse version output of %s: %s", name, res.Stdout)
		}
		results = append(results, probe{name, version})
	}
	if !strings.HasPrefix(results[0].version, "18.") {
		return "", diagnostics.OSUnsupported("archive is PostgreSQL %s, not a supported 18.x build", results[0].version)
	}
	if results[0].version != results[1].version || results[1].version != results[2].version {
		return "", diagnostics.RuntimeMissing("version mismatch across core binaries: postgres=%s initdb=%s pg_ctl=%s",
			results[0].version, results[1].version, results[2].version)
	}
	return results[0].version, nil
}

// parsePostgresVersion extracts the leading "X.Y.Z" from output like
// "postgres (PostgreSQL) 18.6".
func parsePostgresVersion(out string) (string, error) {
	fields := strings.Fields(out)
	for _, f := range fields {
		trimmed := strings.TrimRight(f, ",;")
		if i := strings.Index(trimmed, "."); i > 0 {
			// Match a dotted numeric token like 18.6 or 18.6.1.
			if allDigits(trimmed) {
				return trimmed, nil
			}
		}
	}
	return "", fmt.Errorf("no version token in %q", out)
}

func allDigits(s string) bool {
	// Allow digits and dots only.
	for _, c := range s {
		if (c < '0' || c > '9') && c != '.' {
			return false
		}
	}
	return s != "" && strings.Contains(s, ".")
}

// CheckArchitecture verifies the core EXEs are x64 (IMAGE_FILE_MACHINE_AMD64)
// using debug/pe (section 5.3, layer 3).
func CheckArchitecture(binDir string) error {
	for _, name := range coreExecutables {
		if err := checkPE(filepath.Join(binDir, name)); err != nil {
			return err
		}
	}
	return nil
}

func checkPE(path string) error {
	f, err := pe.Open(path)
	if err != nil {
		return diagnostics.RuntimeMissing("could not inspect PE headers of %s: %v", filepath.Base(path), err)
	}
	defer f.Close()
	switch f.Machine {
	case pe.IMAGE_FILE_MACHINE_AMD64:
		return nil
	case pe.IMAGE_FILE_MACHINE_I386:
		return diagnostics.OSUnsupported("%s is a 32-bit x86 binary; only x64 is supported", filepath.Base(path))
	default:
		return diagnostics.OSUnsupported("%s has unexpected machine type 0x%x; only x64 is supported", filepath.Base(path), f.Machine)
	}
}

// MissingRuntimeFiles returns the list of core binaries missing from binDir.
func MissingRuntimeFiles(binDir string) []string {
	var missing []string
	for _, name := range coreExecutables {
		if _, err := os.Stat(filepath.Join(binDir, name)); err != nil {
			missing = append(missing, name)
		}
	}
	return missing
}
