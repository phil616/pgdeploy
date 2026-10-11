package testutil

// FakeElevator is an in-memory winservice.Elevator for tests. It records the
// arguments it was asked to elevate with.
type FakeElevator struct {
	Elevated     bool
	ElevateCode  int
	ElevateErr   error
	ElevateArgs  []string
	ElevateCalls int
}

func (e *FakeElevator) IsElevated() bool { return e.Elevated }

func (e *FakeElevator) Elevate(args []string) (int, error) {
	e.ElevateCalls++
	e.ElevateArgs = append([]string(nil), args...)
	return e.ElevateCode, e.ElevateErr
}
