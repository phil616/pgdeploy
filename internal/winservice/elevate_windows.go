//go:build windows

package winservice

import (
	"os"
	"strings"
	"syscall"
	"unsafe"

	"github.com/pgdeploy/pgdeploy/internal/diagnostics"
	"golang.org/x/sys/windows"
)

// Windows error codes relevant to elevation.
const (
	errorCancelled        = syscall.Errno(1223) // ERROR_CANCELLED (UAC declined)
	seeMaskNoCloseProcess = 0x00000040
	swShowDefault         = 10
	waitInfinite          = 0xFFFFFFFF
)

// shellExecuteInfo mirrors SHELLEXECUTEINFOW for amd64 (112 bytes).
type shellExecuteInfo struct {
	cbSize       uint32
	fMask        uint32
	hwnd         uintptr
	lpVerb       *uint16
	lpFile       *uint16
	lpParameters *uint16
	lpDirectory  *uint16
	nShow        int32
	_            uint32
	hInstApp     uintptr
	lpIDList     uintptr
	lpClass      *uint16
	hkeyClass    uintptr
	dwHotKey     uint32
	_            uint32
	hIcon        uintptr
	hProcess     uintptr
}

// ElevatorImpl is the production UAC elevation helper.
type ElevatorImpl struct{}

// NewElevator returns the real elevator.
func NewElevator() *ElevatorImpl { return &ElevatorImpl{} }

// IsElevated checks the current process token elevation state (not a
// forgeable flag).
func (e *ElevatorImpl) IsElevated() bool {
	return windows.GetCurrentProcessToken().IsElevated()
}

// Elevate relaunches the current executable with the runas verb and the
// original arguments, waits for the child, and returns its exit code. It does
// not depend on cmd.exe or PowerShell.
func (e *ElevatorImpl) Elevate(args []string) (int, error) {
	exe, err := os.Executable()
	if err != nil {
		return 0, diagnostics.Wrap(diagnostics.Internal("cannot locate own executable for elevation"), err)
	}

	verb, _ := windows.UTF16PtrFromString("runas")
	file, _ := windows.UTF16PtrFromString(exe)
	params := quoteArgs(args)
	var paramPtr *uint16
	if params != "" {
		paramPtr, _ = windows.UTF16PtrFromString(params)
	}

	cwd, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	directory, err := windows.UTF16PtrFromString(cwd)
	if err != nil {
		return 0, err
	}
	info := shellExecuteInfo{
		cbSize:       uint32(unsafe.Sizeof(shellExecuteInfo{})),
		fMask:        seeMaskNoCloseProcess,
		lpVerb:       verb,
		lpFile:       file,
		lpParameters: paramPtr,
		lpDirectory:  directory,
		nShow:        swShowDefault,
	}

	proc := syscall.NewLazyDLL("shell32.dll").NewProc("ShellExecuteExW")
	r1, _, lastErr := proc.Call(uintptr(unsafe.Pointer(&info)))
	if r1 == 0 {
		if lastErr == errorCancelled {
			return 0, diagnostics.ElevationDenied("administrator authorization was cancelled by the user")
		}
		return 0, diagnostics.Wrap(diagnostics.ElevationDenied(
			"failed to request administrator elevation"), lastErr)
	}

	// Wait for the elevated child to finish and forward its exit code.
	wait, err := windows.WaitForSingleObject(windows.Handle(info.hProcess), waitInfinite)
	if err != nil || wait == 0xFFFFFFFF {
		windows.CloseHandle(windows.Handle(info.hProcess))
		return 0, diagnostics.Wrap(diagnostics.Internal("failed waiting for elevated child"), err)
	}
	var code uint32
	if err := windows.GetExitCodeProcess(windows.Handle(info.hProcess), &code); err != nil {
		windows.CloseHandle(windows.Handle(info.hProcess))
		return 0, diagnostics.Wrap(diagnostics.Internal("failed reading elevated child exit code"), err)
	}
	windows.CloseHandle(windows.Handle(info.hProcess))
	return int(code), nil
}

// quoteArgs re-quotes each argument so the elevated child receives the
// original argument vector verbatim.
func quoteArgs(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, a := range args {
		quoted = append(quoted, syscall.EscapeArg(a))
	}
	return strings.Join(quoted, " ")
}
