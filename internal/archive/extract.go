package archive

import (
	"archive/zip"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// NewStagingDir creates a unique, exclusively-owned staging directory under
// parent. It returns the created path. The directory is created with
// O_EXCL-equivalent semantics (os.Mkdir fails if the name already exists).
func NewStagingDir(parent string) (string, error) {
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return "", err
	}
	for i := 0; i < 100; i++ {
		name := filepath.Join(parent, "staging-"+randomHex(8))
		if err := os.Mkdir(name, 0o700); err == nil {
			return name, nil
		} else if !os.IsExist(err) {
			return "", err
		}
	}
	return "", fmt.Errorf("could not allocate a unique staging directory under %s", parent)
}

// Extract safely extracts the archive into destDir, which must not already
// exist. It strips a single top-level "pgsql/" prefix so the result is always
// the canonical runtime tree. Every entry is re-validated and constrained to
// remain inside destDir; files are created with O_EXCL to defeat TOCTOU
// races against duplicate entries.
func Extract(zipPath, destDir string) error {
	ins, err := Inspect(zipPath)
	if err != nil {
		return err
	}

	if err := os.Mkdir(destDir, 0o700); err != nil {
		return fmt.Errorf("staging target %s already exists: %w", destDir, err)
	}

	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		return diagnostics.Wrap(diagnostics.ZIPInvalid("cannot open archive %s", zipPath), err)
	}
	defer zr.Close()

	for _, f := range zr.File {
		norm, err := normalizeEntryName(f.Name)
		if err != nil {
			return diagnostics.Wrap(diagnostics.ArchiveUnsafe("unsafe archive entry %q", f.Name), err)
		}
		norm = stripPrefix(norm, ins.TopLevelPrefix)

		rel, err := filepath.Rel(destDir, filepath.Join(destDir, filepath.FromSlash(norm)))
		if err != nil {
			return diagnostics.ArchiveUnsafe("entry %q resolves outside the staging directory", f.Name)
		}
		if rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return diagnostics.ArchiveUnsafe("entry %q escapes the staging directory", f.Name)
		}

		target := filepath.Join(destDir, filepath.FromSlash(norm))
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o700); err != nil {
				return err
			}
			continue
		}

		if err := os.MkdirAll(filepath.Dir(target), 0o700); err != nil {
			return err
		}
		if err := writeEntry(f, target); err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(f *zip.File, target string) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()

	out, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("create %s: %w", target, err)
	}

	// Copy exactly the declared size plus one sentinel byte; if the stream
	// yields more than declared we abort instead of trusting the central
	// directory claim (section 5.2).
	limited := io.LimitReader(rc, int64(f.UncompressedSize64)+1)
	n, err := io.Copy(out, limited)
	closeErr := out.Close()
	if err != nil {
		return fmt.Errorf("extract %s: %w", f.Name, err)
	}
	if closeErr != nil {
		return closeErr
	}
	if n > int64(f.UncompressedSize64) {
		os.Remove(target)
		return diagnostics.ArchiveUnsafe("entry %q decompresses to more than its declared size", f.Name)
	}
	return nil
}

// stripPrefix removes a single leading "pgsql/" if present.
func stripPrefix(norm, prefix string) string {
	if prefix != "" && strings.HasPrefix(norm, prefix) {
		return strings.TrimPrefix(norm, prefix)
	}
	return norm
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err) // crypto/rand failure is unrecoverable
	}
	return hex.EncodeToString(b)
}
