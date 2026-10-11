//go:build windows

package state

import (
	"errors"
	"fmt"
	"os"
	"testing"
)

func TestLockContentionAndRelease(t *testing.T) {
	key := fmt.Sprintf("PGDeployTest%d", os.Getpid())
	release, err := AcquireInstallLock(key)
	if err != nil {
		t.Fatal(err)
	}
	defer release()
	for i := 0; i < 3; i++ {
		if unlock, err := AcquireInstallLock(key); !errors.Is(err, ErrInstallLocked) {
			if unlock != nil {
				unlock()
			}
			t.Fatalf("contention: %v", err)
		}
	}
	release()
	again, err := AcquireInstallLock(key)
	if err != nil {
		t.Fatalf("lock leaked after release: %v", err)
	}
	again()
	rootRelease, err := AcquireRootLock(`C:\ProgramData\PGDeploy\test`)
	if err != nil {
		t.Fatal(err)
	}
	rootRelease()
}
