// Package archive implements ZIP discovery, safety inspection, and secure
// extraction for PostgreSQL EDB Windows binary archives. It is pure Go and
// cross-platform testable.
package archive

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// zipNamePattern matches the supported archive: PostgreSQL 18.x, EDB,
// Windows x64 binaries ZIP (section 5.1). Extension case is ignored.
var zipNamePattern = regexp.MustCompile(`(?i)^postgresql-18\.[0-9]+-[0-9]+-windows-x64-binaries\.zip$`)

// IsSupportedZipName reports whether the base file name matches the supported
// archive pattern.
func IsSupportedZipName(name string) bool {
	return zipNamePattern.MatchString(filepath.Base(name))
}

// DiscoverZIPs scans dir (the executable's own directory, not the CWD) for
// supported archives. It returns the matching paths sorted for determinism.
func DiscoverZIPs(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var matches []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if zipNamePattern.MatchString(e.Name()) {
			matches = append(matches, filepath.Join(dir, e.Name()))
		}
	}
	return matches, nil
}

// SelectZIP enforces the single-archive rule: zero matches is
// E_ZIP_NOT_FOUND, more than one is E_ZIP_AMBIGUOUS (requiring --zip). It
// never silently picks the highest version.
func SelectZIP(dir, explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", diagnostics.Wrap(diagnostics.ZIPInvalid("could not resolve --zip path"), err)
		}
		if !IsSupportedZipName(abs) {
			return "", diagnostics.ZIPInvalid("--zip does not match the supported PostgreSQL 18 EDB Windows x64 archive name: %s", filepath.Base(abs))
		}
		if _, err := os.Stat(abs); err != nil {
			return "", diagnostics.Wrap(diagnostics.ZIPNotFound("--zip file not found: %s", abs), err)
		}
		return abs, nil
	}

	matches, err := DiscoverZIPs(dir)
	if err != nil {
		return "", diagnostics.Wrap(diagnostics.ZIPNotFound("could not scan executable directory %s", dir), err)
	}
	switch len(matches) {
	case 0:
		return "", diagnostics.ZIPNotFound("no supported PostgreSQL 18 archive found in %s; provide --zip", dir)
	case 1:
		return matches[0], nil
	default:
		return "", diagnostics.ZIPAmbiguous(
			"%d supported archives found in %s (%s); specify exactly one with --zip",
			len(matches), dir, strings.Join(basenames(matches), ", "),
		)
	}
}

func basenames(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = filepath.Base(p)
	}
	return out
}
