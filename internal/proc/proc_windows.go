//go:build windows

package proc

import (
	"os/exec"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

type jobHandle windows.Handle

func (m *Manager) initJob() error {
	h, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return err
	}
	var info windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	info.BasicLimitInformation.LimitFlags = windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE
	_, err = windows.SetInformationJobObject(
		h,
		windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&info)),
		uint32(unsafe.Sizeof(info)),
	)
	if err != nil {
		_ = windows.CloseHandle(h)
		return err
	}
	m.job = jobHandle(h)
	return nil
}

func (m *Manager) assign(cmd *exec.Cmd) error {
	if m.job == 0 || cmd.Process == nil {
		return nil
	}
	const PROCESS_SET_QUOTA = 0x0100
	const PROCESS_TERMINATE = 0x0001
	const PROCESS_ALL = PROCESS_SET_QUOTA | PROCESS_TERMINATE | windows.PROCESS_QUERY_INFORMATION
	h, err := windows.OpenProcess(PROCESS_ALL, false, uint32(cmd.Process.Pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	return windows.AssignProcessToJobObject(windows.Handle(m.job), h)
}

func (m *Manager) closeJob() {
	if m.job != 0 {
		_ = windows.CloseHandle(windows.Handle(m.job))
		m.job = 0
	}
}

func prepareCmd(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CreationFlags: syscall.CREATE_NEW_PROCESS_GROUP,
	}
	return nil
}

func terminate(cmd *exec.Cmd) error {
	// os.Interrupt is not supported via Process.Signal on Windows.
	return killForce(cmd)
}

func killForce(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
