//go:build windows

package lock

import (
	"os"
	"syscall"
)

func signalZero(p *os.Process) bool {
	// On Windows, FindProcess always succeeds; OpenProcess to check.
	h, err := syscall.OpenProcess(syscall.PROCESS_QUERY_INFORMATION, false, uint32(p.Pid))
	if err != nil {
		return false
	}
	_ = syscall.CloseHandle(h)
	return true
}
