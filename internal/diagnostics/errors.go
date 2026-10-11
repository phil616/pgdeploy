// Package diagnostics provides structured error codes and the JSON Lines
// installer logger shared by every other package.
package diagnostics

import (
	"errors"
	"fmt"
)

// Exit codes from the specification (section 14).
const (
	ExitOK        = 0
	ExitUsage     = 2
	ExitElevation = 10
	ExitZIP       = 20
	ExitArchive   = 21
	ExitHash      = 22
	ExitOS        = 30
	ExitTarget    = 31
	ExitService   = 32
	ExitRuntime   = 40
	ExitACLSCM    = 41
	ExitHealth    = 42
	ExitRecovery  = 50
	ExitInternal  = 70
)

// Error codes from the specification (section 14).
const (
	CodeUsage            = "E_USAGE"
	CodeElevationDenied  = "E_ELEVATION_DENIED"
	CodeZIPNotFound      = "E_ZIP_NOT_FOUND"
	CodeZIPAmbiguous     = "E_ZIP_AMBIGUOUS"
	CodeArchiveUnsafe    = "E_ARCHIVE_UNSAFE"
	CodeZIPInvalid       = "E_ZIP_INVALID"
	CodeHashMismatch     = "E_HASH_MISMATCH"
	CodeSourceUnverified = "E_SOURCE_UNVERIFIED"
	CodeOSUnsupported    = "E_OS_UNSUPPORTED"
	CodeFSUnsupported    = "E_FILESYSTEM_UNSUPPORTED"
	CodeTargetOccupied   = "E_TARGET_OCCUPIED"
	CodeDataDirOccupied  = "E_DATA_DIR_OCCUPIED"
	CodeServiceConflict  = "E_SERVICE_CONFLICT"
	CodeInstallLocked    = "E_INSTALL_LOCKED"
	CodeRuntimeMissing   = "E_RUNTIME_MISSING"
	CodeInitDBFailed     = "E_INITDB_FAILED"
	CodeACLFailed        = "E_ACL_FAILED"
	CodeSCMFailed        = "E_SCM_FAILED"
	CodeHealthFailed     = "E_HEALTHCHECK_FAILED"
	CodePortOccupied     = "E_PORT_OCCUPIED"
	CodeRecoveryReview   = "E_RECOVERY_MANUAL_REVIEW"
	CodeInternal         = "E_INTERNAL"
)

// Warnings surfaced by the verify/doctor path but fatal for install.
const (
	WarnSourceAuthenticityUnverified = "SOURCE_AUTHENTICITY_UNVERIFIED"
)

// Error is the structured error type used across the installer. Code and
// Exit map directly onto the table in the specification (section 14);
// Detail carries a redacted, human-actionable description; Recoverable
// tells the caller whether a later re-run or `repair` can recover.
type Error struct {
	Code        string
	Message     string
	Detail      string
	Exit        int
	Recoverable bool
	Cause       error
}

func (e *Error) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Detail != "" {
		return fmt.Sprintf("%s: %s (%s)", e.Code, e.Message, e.Detail)
	}
	return fmt.Sprintf("%s: %s", e.Code, e.Message)
}

// Unwrap exposes the underlying cause for errors.Is/errors.As.
func (e *Error) Unwrap() error { return e.Cause }

// New builds a structured error.
func New(code, message string, exit int, recoverable bool) *Error {
	return &Error{Code: code, Message: message, Exit: exit, Recoverable: recoverable}
}

// Wrap attaches an underlying cause, using its text as Detail when no
// detail has been set. The cause text must already be secret-free.
func Wrap(e *Error, cause error) *Error {
	if e == nil {
		return New(CodeInternal, "nil error wrapped", ExitInternal, false)
	}
	e.Cause = cause
	if cause != nil && e.Detail == "" {
		e.Detail = cause.Error()
	}
	return e
}

// WithDetail sets a human-readable detail string (must be secret-free).
func (e *Error) WithDetail(format string, args ...any) *Error {
	e.Detail = fmt.Sprintf(format, args...)
	return e
}

// ExitCode maps an arbitrary error to a process exit code.
func ExitCode(err error) int {
	if err == nil {
		return ExitOK
	}
	var de *Error
	if errors.As(err, &de) {
		return de.Exit
	}
	return ExitInternal
}

// Convenience constructors for every code in the specification table.

func Usage(format string, args ...any) *Error {
	return New(CodeUsage, fmt.Sprintf(format, args...), ExitUsage, false)
}

func ElevationDenied(format string, args ...any) *Error {
	return New(CodeElevationDenied, fmt.Sprintf(format, args...), ExitElevation, false)
}

func ZIPNotFound(format string, args ...any) *Error {
	return New(CodeZIPNotFound, fmt.Sprintf(format, args...), ExitZIP, false)
}

func ZIPAmbiguous(format string, args ...any) *Error {
	return New(CodeZIPAmbiguous, fmt.Sprintf(format, args...), ExitZIP, false)
}

func ArchiveUnsafe(format string, args ...any) *Error {
	return New(CodeArchiveUnsafe, fmt.Sprintf(format, args...), ExitArchive, false)
}

func ZIPInvalid(format string, args ...any) *Error {
	return New(CodeZIPInvalid, fmt.Sprintf(format, args...), ExitArchive, false)
}

func HashMismatch(format string, args ...any) *Error {
	return New(CodeHashMismatch, fmt.Sprintf(format, args...), ExitHash, false)
}

func SourceUnverified(format string, args ...any) *Error {
	return New(CodeSourceUnverified, fmt.Sprintf(format, args...), ExitHash, false)
}

func OSUnsupported(format string, args ...any) *Error {
	return New(CodeOSUnsupported, fmt.Sprintf(format, args...), ExitOS, false)
}

func FSUnsupported(format string, args ...any) *Error {
	return New(CodeFSUnsupported, fmt.Sprintf(format, args...), ExitOS, false)
}

func TargetOccupied(format string, args ...any) *Error {
	return New(CodeTargetOccupied, fmt.Sprintf(format, args...), ExitTarget, false)
}

func DataDirOccupied(format string, args ...any) *Error {
	return New(CodeDataDirOccupied, fmt.Sprintf(format, args...), ExitTarget, false)
}

func ServiceConflict(format string, args ...any) *Error {
	return New(CodeServiceConflict, fmt.Sprintf(format, args...), ExitService, false)
}

func InstallLocked(format string, args ...any) *Error {
	return New(CodeInstallLocked, fmt.Sprintf(format, args...), ExitService, false)
}

func RuntimeMissing(format string, args ...any) *Error {
	return New(CodeRuntimeMissing, fmt.Sprintf(format, args...), ExitRuntime, true)
}

func InitDBFailed(format string, args ...any) *Error {
	return New(CodeInitDBFailed, fmt.Sprintf(format, args...), ExitRuntime, false)
}

func ACLFailed(format string, args ...any) *Error {
	return New(CodeACLFailed, fmt.Sprintf(format, args...), ExitACLSCM, false)
}

func SCMFailed(format string, args ...any) *Error {
	return New(CodeSCMFailed, fmt.Sprintf(format, args...), ExitACLSCM, false)
}

func HealthFailed(format string, args ...any) *Error {
	return New(CodeHealthFailed, fmt.Sprintf(format, args...), ExitHealth, false)
}

func PortOccupied(format string, args ...any) *Error {
	return New(CodePortOccupied, fmt.Sprintf(format, args...), ExitHealth, false)
}

func RecoveryReview(format string, args ...any) *Error {
	return New(CodeRecoveryReview, fmt.Sprintf(format, args...), ExitRecovery, true)
}

func Internal(format string, args ...any) *Error {
	return New(CodeInternal, fmt.Sprintf(format, args...), ExitInternal, false)
}
