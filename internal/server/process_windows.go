//go:build windows

package server

import "syscall"

const (
	processQueryLimitedInformation = 0x1000
	stillActive                    = 259
)

// processAlive reports whether a process with the given PID
// exists. On Windows, signal-0 is not supported, so we open
// a process handle and check whether it has exited.
func processAlive(pid int) bool {
	h, err := syscall.OpenProcess(
		processQueryLimitedInformation, false, uint32(pid),
	)
	if err != nil {
		return false
	}
	defer syscall.CloseHandle(h)

	var exitCode uint32
	err = syscall.GetExitCodeProcess(h, &exitCode)
	if err != nil {
		return false
	}
	// STILL_ACTIVE (259) means the process has not exited.
	return exitCode == stillActive
}
