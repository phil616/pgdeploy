//go:build release

package source

import "testing"

// TestReleaseAllowlistNonEmpty is the release gate (section 5.3): a customer
// release MUST carry an audited, non-empty allowlist. It only runs under
// -tags release, and fails otherwise, so an empty allowlist can never be
// shipped as a "default install works" build.
func TestReleaseAllowlistNonEmpty(t *testing.T) {
	if len(Allowlist) == 0 {
		t.Fatal("release build requires a non-empty source allowlist; audit the official archive SHA-256 before shipping")
	}
	for name, hash := range Allowlist {
		if len(hash) != 64 {
			t.Fatalf("allowlist entry %q has invalid SHA-256 length %d", name, len(hash))
		}
	}
}
