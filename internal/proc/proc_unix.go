//go:build !windows

package proc

import (
	"os/exec"
	"syscall"
)

type jobHandle int

func (m *Manager) initJob() error { return nil }

func (m *Manager) assign(cmd *exec.Cmd) error { return nil }

func (m *Manager) closeJob() {}

func prepareCmd(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	return nil
}

func terminate(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return cmd.Process.Signal(syscall.SIGTERM)
	}
	return syscall.Kill(-pgid, syscall.SIGTERM)
}

func killForce(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid, err := syscall.Getpgid(cmd.Process.Pid)
	if err != nil {
		return cmd.Process.Kill()
	}
	return syscall.Kill(-pgid, syscall.SIGKILL)
}
