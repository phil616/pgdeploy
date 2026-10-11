package testutil

import (
	"errors"
	"sync"

	"github.com/pgdeploy/pgdeploy/internal/winservice"
)

// FakeServiceManager is an in-memory winservice.ServiceManager for tests. It
// models the SCM semantics that install/repair/uninstall rely on: duplicate
// Create fails, Delete is idempotent, and Query reflects Start/Stop.
type FakeServiceManager struct {
	mu       sync.Mutex
	services map[string]winservice.ServiceSpec
	states   map[string]winservice.ServiceStatus
}

// NewFakeServiceManager returns an empty fake SCM.
func NewFakeServiceManager() *FakeServiceManager {
	return &FakeServiceManager{
		services: map[string]winservice.ServiceSpec{},
		states:   map[string]winservice.ServiceStatus{},
	}
}

func (f *FakeServiceManager) Create(spec winservice.ServiceSpec) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.services[spec.Name]; ok {
		return errors.New("service already exists")
	}
	f.services[spec.Name] = spec
	f.states[spec.Name] = winservice.ServiceStatus{State: winservice.StateStopped}
	return nil
}

func (f *FakeServiceManager) Start(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.services[name]; !ok {
		return errors.New("service does not exist")
	}
	f.states[name] = winservice.ServiceStatus{State: winservice.StateRunning, ProcessID: 1234}
	return nil
}

func (f *FakeServiceManager) Stop(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.services[name]; !ok {
		return errors.New("service does not exist")
	}
	f.states[name] = winservice.ServiceStatus{State: winservice.StateStopped}
	return nil
}

func (f *FakeServiceManager) Query(name string) (winservice.ServiceStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.states[name]; ok {
		return s, nil
	}
	return winservice.ServiceStatus{}, errors.New("service does not exist")
}

func (f *FakeServiceManager) Config(name string) (winservice.ServiceSpec, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if s, ok := f.services[name]; ok {
		return s, nil
	}
	return winservice.ServiceSpec{}, errors.New("service does not exist")
}

func (f *FakeServiceManager) Delete(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.services, name)
	delete(f.states, name)
	return nil
}

func (f *FakeServiceManager) Exists(name string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, ok := f.services[name]
	return ok, nil
}

func (f *FakeServiceManager) SetRecoveryActions(name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if _, ok := f.services[name]; !ok {
		return errors.New("service does not exist")
	}
	return nil
}

func (f *FakeServiceManager) SetStartType(name string, startType int) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.services[name]
	if !ok {
		return errors.New("service does not exist")
	}
	s.StartType = startType
	f.services[name] = s
	return nil
}
