// Package opts holds the resolved command-line options shared across the
// planner, installer, and CLI. It is pure Go and cross-platform testable.
package opts

// Options carries the resolved CLI inputs for a single command (section 4).
type Options struct {
	ZipPath        string
	RootPath       string
	ServiceName    string
	PasswordFile   string
	ExpectedSHA256 string
	Port           int
	ManualStart    bool
	NoStart        bool
	Verbose        bool
	JSON           bool
	AssumeYes      bool
}
