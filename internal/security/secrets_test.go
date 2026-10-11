package security

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGeneratePassword(t *testing.T) {
	pw, err := GeneratePassword()
	if err != nil {
		t.Fatalf("GeneratePassword: %v", err)
	}
	if len(pw) != 43 {
		t.Errorf("password length = %d, want 43", len(pw))
	}
	if strings.Contains(pw, "=") {
		t.Errorf("password contains base64 padding: %q", pw)
	}
	// Should be URL-safe base64 (no + or /).
	if strings.ContainsAny(pw, "+/") {
		t.Errorf("password contains non-URL-safe base64 characters: %q", pw)
	}
}

func TestGeneratePasswordUnique(t *testing.T) {
	a, _ := GeneratePassword()
	b, _ := GeneratePassword()
	if a == b {
		t.Fatal("two generated passwords were identical")
	}
}

func TestValidatePasswordContent(t *testing.T) {
	valid := []string{"s3cret", "p@ss w0rd", "汉字密码"}
	for _, pw := range valid {
		if err := ValidatePasswordContent(pw); err != nil {
			t.Errorf("ValidatePasswordContent(%q) = %v, want nil", pw, err)
		}
	}
	invalid := []string{"", "line\nbreak", "line\rbreak", strings.Repeat("x", 1025)}
	for _, pw := range invalid {
		if err := ValidatePasswordContent(pw); err == nil {
			t.Errorf("ValidatePasswordContent(%q) = nil, want error", pw)
		}
	}
}

func TestReadPasswordFileFirstLineOnly(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "pw.txt")
	if err := os.WriteFile(p, []byte("hunter2\nsecond-line-ignored\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := ReadPasswordFile(p)
	if err != nil {
		t.Fatalf("ReadPasswordFile: %v", err)
	}
	if got != "hunter2" {
		t.Errorf("got %q, want %q", got, "hunter2")
	}
}

func TestWriteSecretFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	p, err := WriteSecretFile(dir, "postgres-password.txt", []byte("secret\n"))
	if err != nil {
		t.Fatalf("WriteSecretFile: %v", err)
	}
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "secret\n" {
		t.Errorf("content = %q, want %q", string(b), "secret\n")
	}
}
