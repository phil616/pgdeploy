package postgres

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestVersionNumbers(t *testing.T) {
	for input, want := range map[string]string{"180000": "18.0", "180006": "18.6", "180012": "18.12", "170005": "17.5", "bad": "bad"} {
		if got := FormatVersionNum(input); got != want {
			t.Errorf("%s => %s, want %s", input, got, want)
		}
	}
}

func TestSanitizedEnv(t *testing.T) {
	t.Setenv("PGHOSTADDR", "192.0.2.1")
	t.Setenv("PGSERVICEFILE", "untrusted.conf")
	t.Setenv("PGPASSWORD", "inherited")
	env := sanitizedEnv("PGPASSWORD=explicit")
	for _, kv := range env {
		if strings.HasPrefix(strings.ToUpper(kv), "PG") && kv != "PGPASSWORD=explicit" {
			t.Errorf("inherited PostgreSQL option: %s", kv)
		}
	}
}

func TestIncompleteClusterRejected(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("18\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := ValidateDataDirectory(dir); err == nil {
		t.Fatal("accepted PG_VERSION without pg_control")
	}
}
