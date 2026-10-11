//go:build windows

package security

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// Access masks used in SDDL strings. See the permission matrix in the
// specification (section 7.2).
const (
	maskFull     = "FA"       // FILE_ALL_ACCESS
	maskReadExec = "0x1200A9" // FILE_GENERIC_READ | FILE_GENERIC_EXECUTE
	maskModify   = "0x1301BF" // read/write/execute + delete (tree modify)
	maskTraverse = "0x20"     // FILE_TRAVERSE
)

// ACLRole selects which directory role's DACL to build.
type ACLRole int

const (
	// RoleRoot covers the instance root and state directory: admins get full
	// control, the service account gets traverse-only access.
	RoleRoot ACLRole = iota
	// RoleRuntime covers runtime\pgsql: service gets read+execute.
	RoleRuntime
	// RoleData covers the data directory: service gets modify.
	RoleData
	// RoleLogsPostgres covers logs\postgres: service gets modify.
	RoleLogsPostgres
	// RoleLogsDir covers the logs directory itself: service gets traverse to
	// reach logs\postgres but cannot read the installer log.
	RoleLogsDir
	// RoleSecrets covers secrets: no service access at all.
	RoleSecrets
	// RoleStaging covers the temporary staging directory: admin-only.
	RoleStaging
)

// sddlFor builds the protected-DACL SDDL string for a role.
func sddlFor(role ACLRole, serviceSID string) string {
	sysAdmin := "(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)"
	switch role {
	case RoleRoot, RoleLogsDir:
		return "D:P" + sysAdmin + "(A;;0x20;;;" + serviceSID + ")"
	case RoleRuntime:
		return "D:P" + sysAdmin + "(A;OICI;0x1200A9;;;" + serviceSID + ")"
	case RoleData, RoleLogsPostgres:
		return "D:P" + sysAdmin + "(A;OICI;0x1301BF;;;" + serviceSID + ")"
	case RoleSecrets, RoleStaging:
		return "D:P" + sysAdmin
	default:
		panic("unknown ACL role")
	}
}

// ApplyDirACL replaces the DACL on path with the role's protected DACL. The
// DACL is protected so it does not inherit potentially permissive entries
// from a user-writable parent directory.
func ApplyDirACL(path string, role ACLRole, serviceSID string) error {
	sddl := sddlFor(role, serviceSID)
	sd, err := windows.SecurityDescriptorFromString(sddl)
	if err != nil {
		return fmt.Errorf("parse SDDL for %s: %w", path, err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("extract DACL for %s: %w", path, err)
	}
	err = windows.SetNamedSecurityInfo(
		path,
		windows.SE_FILE_OBJECT,
		windows.DACL_SECURITY_INFORMATION|windows.PROTECTED_DACL_SECURITY_INFORMATION,
		nil, nil, dacl, nil,
	)
	if err != nil {
		return fmt.Errorf("set ACL on %s: %w", path, err)
	}
	return nil
}

// VerifyDirACL reads back the security descriptor on path and checks that its
// DACL is protected from parent inheritance and that the service SID appears
// in (or is absent from) the DACL as the role requires. It returns nil when
// the ACL is consistent with the role.
func VerifyDirACL(path string, role ACLRole, serviceSID string) error {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read ACL on %s: %w", path, err)
	}
	if sd == nil {
		return fmt.Errorf("no security descriptor on %s", path)
	}
	sddl := sd.String()
	if sddl == "" {
		return fmt.Errorf("could not render ACL on %s", path)
	}
	upper := strings.ToUpper(sddl)
	if !strings.Contains(upper, "D:P") && !strings.Contains(upper, "D:PAI") {
		return fmt.Errorf("DACL on %s is not protected from parent inheritance", path)
	}
	hasService := strings.Contains(strings.ToUpper(sddl), strings.ToUpper(serviceSID))
	if role == RoleSecrets || role == RoleStaging {
		if hasService {
			return fmt.Errorf("ACL on %s grants the service account access (should be denied)", path)
		}
		return nil
	}
	if !hasService {
		return fmt.Errorf("ACL on %s does not grant the service account %s access", path, serviceSID)
	}
	return nil
}
