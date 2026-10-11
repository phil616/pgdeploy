//go:build windows

package state

import (
	"crypto/sha256"
	"fmt"
	"strings"
	"sync"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"golang.org/x/sys/windows"
)

var ErrInstallLocked = diagnostics.InstallLocked("another pgdeploy process is operating on this instance")

// The lifetime of the named object is the lock: no thread ownership is used,
// because Go goroutines may migrate between OS threads. Closing the last handle
// (including on process exit) makes the name available again.
func acquireNamedLock(name string) (func(), error) {
	ptr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, ptr)
	if err != nil {
		if h != 0 {
			windows.CloseHandle(h)
		}
		if err == windows.ERROR_ALREADY_EXISTS {
			return nil, ErrInstallLocked
		}
		return nil, err
	}
	var once sync.Once
	return func() { once.Do(func() { windows.CloseHandle(h) }) }, nil
}

func AcquireInstallLock(serviceName string) (func(), error) {
	return acquireNamedLock(`Global\PGDeploy.Install.` + strings.ToUpper(serviceName))
}

func AcquireRootLock(rootKey string) (func(), error) {
	return acquireNamedLock(fmt.Sprintf(`Global\PGDeploy.Root.%x`, sha256.Sum256([]byte(strings.ToUpper(rootKey)))))
}
