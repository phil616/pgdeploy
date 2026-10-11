//go:build windows

package security

import (
	"fmt"

	"golang.org/x/sys/windows"
)

// ServiceAccountName returns the virtual service account name for a given
// service name, e.g. `NT SERVICE\PGDeployPostgreSQL18`.
func ServiceAccountName(serviceName string) string {
	return `NT SERVICE\` + serviceName
}

// ServiceSID resolves the virtual service account SID for a service name.
// This must only be called after the SCM service has been created, because
// the virtual account SID is not guaranteed to resolve before that point.
func ServiceSID(serviceName string) (*windows.SID, error) {
	sid, _, _, err := windows.LookupSID("", ServiceAccountName(serviceName))
	if err != nil {
		return nil, fmt.Errorf("resolve service SID for %q: %w", serviceName, err)
	}
	return sid, nil
}

// Well-known SIDs are embedded in SDDL strings by their short names, so this
// file only needs to expose the service SID as an "S-1-5-80-..." string.
func ServiceSIDString(serviceName string) (string, error) {
	sid, err := ServiceSID(serviceName)
	if err != nil {
		return "", err
	}
	return sid.String(), nil
}

// CurrentUserSIDString identifies the elevated caller independently of its
// administrator group, which initdb disables in its restricted child token.
func CurrentUserSIDString() (string, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", err
	}
	return user.User.Sid.String(), nil
}
