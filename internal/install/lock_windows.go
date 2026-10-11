//go:build windows

package install

import (
	"github.com/pgdeploy/pgdeploy/internal/plan"
	"github.com/pgdeploy/pgdeploy/internal/state"
)

func lockInstance(root, service string) (func(), error) {
	releaseService, err := state.AcquireInstallLock(service)
	if err != nil {
		return nil, err
	}
	releaseRoot, err := state.AcquireRootLock(plan.RootKey(root))
	if err != nil {
		releaseService()
		return nil, err
	}
	return func() { releaseRoot(); releaseService() }, nil
}
