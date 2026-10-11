// Package security handles credential generation and protected storage.
// The pure-Go portions (password generation and file I/O) live here; the
// Windows ACL and SID work lives in acl_windows.go / sid_windows.go.
package security

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

// GeneratedPasswordLen is the number of raw random bytes (32) that produce
// a 43-character Base64 URL-safe no-padding password.
const GeneratedPasswordLen = 32

// GeneratePassword returns a 43-character Base64 URL-safe password derived
// from 32 cryptographically random bytes.
func GeneratePassword() (string, error) {
	b := make([]byte, GeneratedPasswordLen)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// ValidatePasswordContent enforces the rules for a user-supplied password:
// non-empty, valid UTF-8, no embedded newlines, and a sane length.
func ValidatePasswordContent(pw string) error {
	if pw == "" {
		return fmt.Errorf("password is empty")
	}
	if !utf8.ValidString(pw) {
		return fmt.Errorf("password is not valid UTF-8")
	}
	if strings.ContainsAny(pw, "\r\n") {
		return fmt.Errorf("password must not contain newlines")
	}
	if len(pw) > 1024 {
		return fmt.Errorf("password is unreasonably long")
	}
	return nil
}

// ReadPasswordFile reads only the first line of a user-supplied password
// file, trims a single trailing newline, and validates it. The source file is
// never modified or deleted.
func ReadPasswordFile(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	content := string(b)
	if i := strings.IndexAny(content, "\r\n"); i >= 0 {
		content = content[:i]
	}
	if err := ValidatePasswordContent(content); err != nil {
		return "", fmt.Errorf("invalid password file %s: %w", path, err)
	}
	return content, nil
}

// WriteSecretFile atomically writes a secret to dir/name with mode 0600,
// returning the final path. ACL tightening happens in the caller (Windows
// only) so this function stays platform-independent.
func WriteSecretFile(dir, name string, content []byte) (string, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	final := filepath.Join(dir, name)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(content); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, final); err != nil {
		return "", err
	}
	return final, nil
}
