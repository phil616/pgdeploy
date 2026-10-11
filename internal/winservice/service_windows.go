//go:build windows

package winservice

import (
	"strings"
	"time"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
)

// Default stop/start wait timeouts.
const (
	startTimeout = 120 * time.Second
	stopTimeout  = 120 * time.Second
)

// recovery actions from section 10.2.
var recoveryActions = []mgr.RecoveryAction{
	{Type: mgr.ServiceRestart, Delay: 30 * time.Second},
	{Type: mgr.ServiceRestart, Delay: 60 * time.Second},
	{Type: mgr.ServiceRestart, Delay: 300 * time.Second},
}

// Mgr is the production ServiceManager backed by the real SCM.
type Mgr struct{}

// New returns the real SCM-backed manager.
func New() *Mgr { return &Mgr{} }

// Create registers a service whose ImagePath is pg_ctl.exe runservice -N ...
// -D ... -w (section 10). It never shells out to `pg_ctl register`.
func (m *Mgr) Create(spec ServiceSpec) error {
	scm, err := mgr.Connect()
	if err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot connect to the Service Control Manager"), err)
	}
	defer scm.Disconnect()

	cfg := mgr.Config{
		DisplayName:      spec.DisplayName,
		StartType:        uint32(spec.StartType),
		ErrorControl:     mgr.ErrorNormal,
		ServiceStartName: spec.ServiceStartName,
		Description:      spec.Description,
		Dependencies:     spec.Dependencies,
	}
	if spec.SidTypeUnrestricted {
		cfg.SidType = windows.SERVICE_SID_TYPE_UNRESTRICTED
	}

	s, err := scm.CreateService(spec.Name, spec.BinaryPath, cfg, spec.Args...)
	if err != nil {
		if err == windows.ERROR_SERVICE_EXISTS {
			return diagnostics.ServiceConflict("service %q already exists", spec.Name)
		}
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot create service %q", spec.Name), err)
	}
	defer s.Close()

	if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot set recovery actions on %q", spec.Name), err)
	}
	if err := s.SetRecoveryActionsOnNonCrashFailures(false); err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot set recovery policy on %q", spec.Name), err)
	}
	return nil
}

// Start starts the service and waits until it reports Running.
func (m *Mgr) Start(name string) error {
	s, err := m.open(name)
	if err != nil {
		return err
	}
	defer s.Close()

	if err := s.Start(); err != nil && err != windows.ERROR_SERVICE_ALREADY_RUNNING {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot start service %q", name), err)
	}
	if err := waitForState(s, svc.Running, startTimeout); err != nil {
		return diagnostics.Wrap(diagnostics.HealthFailed("service %q did not reach Running state", name), err)
	}
	return nil
}

// Stop sends a normal stop control and waits for Stopped.
func (m *Mgr) Stop(name string) error {
	s, err := m.open(name)
	if err != nil {
		return err
	}
	defer s.Close()

	st, err := s.Control(svc.Stop)
	if err != nil && err != windows.ERROR_SERVICE_NOT_ACTIVE {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot stop service %q", name), err)
	}
	_ = st
	if err := waitForState(s, svc.Stopped, stopTimeout); err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("service %q did not stop", name), err)
	}
	return nil
}

// Query returns the normalized service state.
func (m *Mgr) Query(name string) (ServiceStatus, error) {
	s, err := m.open(name)
	if err != nil {
		return ServiceStatus{}, err
	}
	defer s.Close()
	st, err := s.Query()
	if err != nil {
		return ServiceStatus{}, diagnostics.Wrap(diagnostics.SCMFailed("cannot query service %q", name), err)
	}
	return ServiceStatus{State: normalizeState(st.State), ProcessID: st.ProcessId}, nil
}

// Config reads back the created service's configuration.
func (m *Mgr) Config(name string) (ServiceSpec, error) {
	s, err := m.open(name)
	if err != nil {
		return ServiceSpec{}, err
	}
	defer s.Close()
	c, err := s.Config()
	if err != nil {
		return ServiceSpec{}, diagnostics.Wrap(diagnostics.SCMFailed("cannot read config of %q", name), err)
	}
	spec := ServiceSpec{
		Name:                name,
		DisplayName:         c.DisplayName,
		BinaryPath:          c.BinaryPathName,
		ServiceStartName:    c.ServiceStartName,
		StartType:           int(c.StartType),
		Dependencies:        c.Dependencies,
		Description:         c.Description,
		SidTypeUnrestricted: c.SidType == windows.SERVICE_SID_TYPE_UNRESTRICTED,
	}
	return spec, nil
}

// Delete marks the service for deletion.
func (m *Mgr) Delete(name string) error {
	s, err := m.open(name)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.Delete(); err != nil {
		if err == windows.ERROR_SERVICE_MARKED_FOR_DELETE {
			return diagnostics.SCMFailed("service %q is marked for deletion; retry after pending handles close", name)
		}
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot delete service %q", name), err)
	}
	return nil
}

// Exists reports whether a service with the given name exists.
func (m *Mgr) Exists(name string) (bool, error) {
	s, err := m.open(name)
	if err != nil {
		if strings.Contains(err.Error(), "does not exist") || err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
			return false, nil
		}
		return false, err
	}
	s.Close()
	return true, nil
}

// SetRecoveryActions applies the standard recovery policy.
func (m *Mgr) SetRecoveryActions(name string) error {
	s, err := m.open(name)
	if err != nil {
		return err
	}
	defer s.Close()
	if err := s.SetRecoveryActions(recoveryActions, 86400); err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot set recovery actions on %q", name), err)
	}
	return s.SetRecoveryActionsOnNonCrashFailures(false)
}

// SetStartType changes the service start type without touching other fields.
func (m *Mgr) SetStartType(name string, startType int) error {
	s, err := m.open(name)
	if err != nil {
		return err
	}
	defer s.Close()
	err = windows.ChangeServiceConfig(
		s.Handle,
		windows.SERVICE_NO_CHANGE,
		uint32(startType),
		windows.SERVICE_NO_CHANGE,
		nil, nil, nil, nil, nil, nil, nil,
	)
	if err != nil {
		return diagnostics.Wrap(diagnostics.SCMFailed("cannot change start type of %q", name), err)
	}
	return nil
}

func (m *Mgr) open(name string) (*mgr.Service, error) {
	scm, err := mgr.Connect()
	if err != nil {
		return nil, diagnostics.Wrap(diagnostics.SCMFailed("cannot connect to the Service Control Manager"), err)
	}
	defer scm.Disconnect()
	s, err := scm.OpenService(name)
	if err != nil {
		if err == windows.ERROR_SERVICE_DOES_NOT_EXIST {
			return nil, diagnostics.SCMFailed("service %q does not exist", name)
		}
		return nil, diagnostics.Wrap(diagnostics.SCMFailed("cannot open service %q", name), err)
	}
	return s, nil
}

func waitForState(s *mgr.Service, want svc.State, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for {
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State == want {
			return nil
		}
		if time.Now().After(deadline) {
			return &stateTimeoutErr{state: normalizeState(st.State), want: normalizeState(want)}
		}
		time.Sleep(500 * time.Millisecond)
	}
}

type stateTimeoutErr struct{ state, want string }

func (e *stateTimeoutErr) Error() string {
	return "service stuck in state " + e.state + " (wanted " + e.want + ")"
}

func normalizeState(s svc.State) string {
	switch s {
	case svc.Stopped:
		return StateStopped
	case svc.Running:
		return StateRunning
	case svc.StartPending:
		return StateStartPending
	case svc.StopPending:
		return StateStopPending
	case svc.Paused, svc.PausePending, svc.ContinuePending:
		return StatePaused
	default:
		return StateUnknown
	}
}
