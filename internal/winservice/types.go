// Package winservice manages Windows services through the SCM and handles
// UAC elevation. The interface in this file is cross-platform so install
// logic can be tested with a fake ServiceManager.
package winservice

// Start type constants (Windows service start types).
const (
	StartManual    = 3 // SERVICE_DEMAND_START
	StartAutomatic = 2 // SERVICE_AUTO_START
)

// Service state names (normalized).
const (
	StateStopped      = "stopped"
	StateRunning      = "running"
	StateStartPending = "start_pending"
	StateStopPending  = "stop_pending"
	StatePaused       = "paused"
	StateUnknown      = "unknown"
)

// ServiceSpec describes the service to create.
type ServiceSpec struct {
	Name                string
	DisplayName         string
	BinaryPath          string
	Args                []string
	ServiceStartName    string // e.g. NT SERVICE\PGDeployPostgreSQL18
	StartType           int
	Dependencies        []string
	Description         string
	SidTypeUnrestricted bool
}

// ServiceStatus is a normalized view of SCM state.
type ServiceStatus struct {
	State     string
	ProcessID uint32
}

// ServiceManager is the abstraction over the Windows SCM (section 4).
// Production code uses one Windows implementation; tests use a fake.
type ServiceManager interface {
	Create(spec ServiceSpec) error
	Start(name string) error
	Stop(name string) error
	Query(name string) (ServiceStatus, error)
	Config(name string) (ServiceSpec, error)
	Delete(name string) error
	Exists(name string) (bool, error)
	SetRecoveryActions(name string) error
	SetStartType(name string, startType int) error
}

// Elevator performs a one-shot UAC self-elevation.
type Elevator interface {
	// IsElevated reports whether the current process token is elevated.
	IsElevated() bool
	// Elevate relaunches the current executable with the runas verb and the
	// original arguments, waits for it, and returns its exit code.
	Elevate(args []string) (int, error)
}
