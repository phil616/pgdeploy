//go:build windows

package cli

import (
	"os"

	"golang.org/x/sys/windows"
)

// isTerminal reports whether f is a character-device console handle. It gates
// the interactive secondary confirmation for `uninstall --purge-data`
// (section 13.3): non-interactive callers must be refused.
func isTerminal(f *os.File) bool {
	var mode uint32
	return windows.GetConsoleMode(windows.Handle(f.Fd()), &mode) == nil
}
