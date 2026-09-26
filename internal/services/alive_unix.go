//go:build !windows

package services

import (
	"syscall"
)

func procAlive(pid int) bool {
	err := syscall.Kill(pid, 0)
	return err == nil
}
