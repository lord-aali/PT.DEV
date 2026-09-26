//go:build !windows

package lock

import (
	"os"
	"syscall"
)

func signalZero(p *os.Process) bool {
	err := p.Signal(syscall.Signal(0))
	return err == nil
}
