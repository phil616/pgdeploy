package archive

import (
	"archive/zip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// Hard safety limits from the specification (section 5.2).
const (
	MaxTotalUncompressed  = 4 * 1024 * 1024 * 1024 // 4 GiB
	MaxSingleUncompressed = 1 * 1024 * 1024 * 1024 // 1 GiB
	MaxEntryCount         = 30000
)

// requiredFiles must be present and be regular files in the archive.
var requiredFiles = []string{
	"bin/postgres.exe",
	"bin/initdb.exe",
	"bin/pg_ctl.exe",
	"bin/pg_isready.exe",
	"bin/psql.exe",
}

// requiredDirs must be present in the archive.
var requiredDirs = []string{
	"share",
	"lib",
}

// Inspection is the result of a full read-only archive validation.
type Inspection struct {
	ZipPath          string
	SHA256           string
	TopLevelPrefix   string // "" or "pgsql/"
	EntryCount       int
	UncompressedSize uint64
	CompressedSize   uint64
	MaxEntrySize     uint64
}

// ComputeSHA256 streams the file and returns its lowercase hex SHA-256.
func ComputeSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// Inspect opens the archive and performs the full read-only validation
// (structure, safety, size, required files) before any byte is written. It
// also computes the archive SHA-256.
func Inspect(zipPath string) (*Inspection, error) {
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return nil, diagnostics.Wrap(diagnostics.ZIPInvalid("cannot open archive %s", zipPath), err)
	}
	defer zr.Close()

	ins := &Inspection{ZipPath: zipPath}
	seen := make(map[string]string) // lower-cased normalized path -> original
	present := make(map[string]bool)
	dirPresent := make(map[string]bool)

	for _, f := range zr.File {
		ins.EntryCount++
		if ins.EntryCount > MaxEntryCount {
			return nil, diagnostics.ArchiveUnsafe(
				"archive has more than %d entries; refusing to extract", MaxEntryCount)
		}

		norm, err := normalizeEntryName(f.Name)
		if err != nil {
			return nil, diagnostics.Wrap(diagnostics.ArchiveUnsafe("unsafe archive entry %q", f.Name), err)
		}
		key := strings.ToLower(norm)
		if prev, dup := seen[key]; dup {
			return nil, diagnostics.ArchiveUnsafe(
				"duplicate or case-colliding archive entries %q and %q", prev, f.Name)
		}
		seen[key] = f.Name

		if strings.HasSuffix(norm, "/") {
			dirPresent[strings.TrimSuffix(norm, "/")] = true
			continue
		}
		present[norm] = true
		// ZIP directory entries are optional; infer ancestors from files.
		for parent := path.Dir(norm); parent != "."; parent = path.Dir(parent) {
			dirPresent[parent] = true
		}

		if f.UncompressedSize64 > MaxSingleUncompressed {
			return nil, diagnostics.ArchiveUnsafe(
				"entry %q expands to %d bytes (limit %d)", f.Name, f.UncompressedSize64, int64(MaxSingleUncompressed))
		}
		if f.UncompressedSize64 > ins.MaxEntrySize {
			ins.MaxEntrySize = f.UncompressedSize64
		}
		ins.UncompressedSize += f.UncompressedSize64
		ins.CompressedSize += f.CompressedSize64
		if ins.UncompressedSize > MaxTotalUncompressed {
			return nil, diagnostics.ArchiveUnsafe(
				"archive expands to more than %d bytes; refusing to extract (possible zip bomb)", int64(MaxTotalUncompressed))
		}

		// Reject non-regular file types (symlinks, reparse points, devices).
		mode := f.Mode()
		if !mode.IsRegular() && !mode.IsDir() {
			return nil, diagnostics.ArchiveUnsafe("archive entry %q is not a regular file or directory", f.Name)
		}
	}

	// Determine the top-level prefix and validate required files.
	ins.TopLevelPrefix = detectPrefix(present, dirPresent)
	if err := checkRequired(present, dirPresent, ins.TopLevelPrefix); err != nil {
		return nil, err
	}

	sha, err := ComputeSHA256(zipPath)
	if err != nil {
		return nil, diagnostics.Wrap(diagnostics.ZIPInvalid("cannot hash archive %s", zipPath), err)
	}
	ins.SHA256 = sha
	return ins, nil
}

// detectPrefix returns "pgsql/" if the archive is rooted at a single pgsql
// directory, otherwise "". It is only used for reporting; extraction always
// targets the canonical runtime/pgsql directory regardless of the prefix.
func detectPrefix(present, dirPresent map[string]bool) string {
	// Anchor detection to the required executable, never map iteration order.
	if present["pgsql/bin/postgres.exe"] {
		return "pgsql/"
	}
	return ""
}

// checkRequired verifies the mandatory files/dirs, stripping the prefix.
func checkRequired(present, dirPresent map[string]bool, prefix string) error {
	for _, f := range requiredFiles {
		key := path.Join(prefix, f)
		if !present[key] {
			return diagnostics.ZIPInvalid("archive is missing required file %s", key)
		}
	}
	for _, d := range requiredDirs {
		key := path.Join(prefix, d)
		if !dirPresent[key] {
			return diagnostics.ZIPInvalid("archive is missing required directory %s/", key)
		}
	}
	return nil
}

// normalizeEntryName validates a single archive entry name against the
// Windows path rules and returns the canonical forward-slash relative path.
func normalizeEntryName(name string) (string, error) {
	// Backslashes are treated as path separators in hostile archives.
	clean := strings.ReplaceAll(name, "\\", "/")

	if strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("absolute path")
	}
	if len(clean) >= 2 && clean[1] == ':' {
		return "", fmt.Errorf("drive-letter path")
	}
	if strings.HasPrefix(clean, "//") {
		return "", fmt.Errorf("UNC path")
	}
	if strings.ContainsRune(clean, ':') {
		return "", fmt.Errorf("colon / alternate data stream")
	}
	for _, part := range strings.Split(clean, "/") {
		if part == ".." {
			return "", fmt.Errorf("parent-directory traversal")
		}
		if isReservedDeviceName(part) {
			return "", fmt.Errorf("reserved device name %q", part)
		}
		if strings.HasSuffix(part, " ") || strings.HasSuffix(part, ".") {
			return "", fmt.Errorf("trailing space or dot in segment %q", part)
		}
	}
	return clean, nil
}

// isReservedDeviceName reports whether a single path segment is a Windows
// reserved device name (CON, PRN, AUX, NUL, COM1-9, LPT1-9, optionally with
// an extension), case-insensitively.
func isReservedDeviceName(segment string) bool {
	base := segment
	if i := strings.IndexByte(base, '.'); i >= 0 {
		base = base[:i]
	}
	if base == "" {
		return false
	}
	upper := strings.ToUpper(base)
	switch upper {
	case "CON", "PRN", "AUX", "NUL":
		return true
	}
	if len(upper) == 4 && (upper[:3] == "COM" || upper[:3] == "LPT") {
		if c := upper[3]; c >= '1' && c <= '9' {
			return true
		}
	}
	return false
}
