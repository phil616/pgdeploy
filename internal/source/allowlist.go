// Package source implements the ZIP source-authenticity policy (section 5.3).
package source

import (
	"strings"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
)

// Allowlist maps a supported archive file name to its audited SHA-256.
//
// MAINTAINERS: before shipping a customer release you MUST populate this map
// with the real SHA-256 of the official EDB archive, obtained through an
// independent trusted channel, and the release gate (go test -tags release)
// will then pass. An empty allowlist is intentional for development: install
// refuses to run unverified archives and requires --sha256 instead. Never
// fabricate a hash here.
var Allowlist = map[string]string{
	// "postgresql-18.6-5-windows-x64-binaries.zip": "<64 lowercase hex>",
}

// Label for verified-by-embedded-allowlist.
const LabelEmbedded = "verified_by_embedded_allowlist"

// LabelSuppliedHash is the label for a user-supplied trusted hash.
const LabelSuppliedHash = "verified_by_supplied_hash"

// Result reports the outcome of a source-authenticity decision.
type Result struct {
	Verified bool
	Label    string
	Reason   string
}

// Verify decides whether the archive may be trusted, given its file name,
// computed SHA-256, and an optional operator-supplied expected hash.
func Verify(zipName, computedSHA string, suppliedSHA string) Result {
	computed := strings.ToLower(strings.TrimSpace(computedSHA))
	if suppliedSHA != "" {
		supplied := strings.ToLower(strings.TrimSpace(suppliedSHA))
		if len(supplied) != 64 || !isHex(supplied) {
			return Result{Verified: false, Reason: "supplied --sha256 is not a valid 64-hex string"}
		}
		if supplied == computed {
			return Result{Verified: true, Label: LabelSuppliedHash}
		}
		return Result{Verified: false, Label: "", Reason: "computed SHA-256 does not match the supplied --sha256"}
	}
	if want, ok := Allowlist[zipName]; ok {
		want = strings.ToLower(strings.TrimSpace(want))
		if want == computed {
			return Result{Verified: true, Label: LabelEmbedded}
		}
		return Result{Verified: false, Reason: "computed SHA-256 does not match the embedded allowlist"}
	}
	return Result{Verified: false, Label: diagnostics.WarnSourceAuthenticityUnverified,
		Reason: "no trusted hash available: provide --sha256 from an independent channel, or build a release with an audited allowlist"}
}

// ErrForInstall converts an unverified result into the fatal install error.
func ErrForInstall(r Result) *diagnostics.Error {
	if r.Verified {
		return nil
	}
	if strings.Contains(r.Reason, "does not match") {
		return diagnostics.HashMismatch("%s", r.Reason)
	}
	return diagnostics.SourceUnverified("%s", r.Reason)
}

func isHex(s string) bool {
	for _, c := range s {
		if (c < '0' || c > '9') && (c < 'a' || c > 'f') {
			return false
		}
	}
	return true
}
