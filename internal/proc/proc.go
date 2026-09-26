package proc

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sync"
)

// Manager tracks child processes and kills them on Close.
type Manager struct {
	mu     sync.Mutex
	cmds   []*exec.Cmd
	job    jobHandle
	closed bool
}

// New creates a Manager. On Windows this creates a Job Object with KILL_ON_JOB_CLOSE.
func New() (*Manager, error) {
	m := &Manager{}
	if err := m.initJob(); err != nil {
		return nil, err
	}
	return m, nil
}

// Start starts cmd as a managed child (assigned to process group / job).
func (m *Manager) Start(cmd *exec.Cmd) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return fmt.Errorf("process manager already closed")
	}
	if err := prepareCmd(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	if err := m.assign(cmd); err != nil {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
		return err
	}
	m.cmds = append(m.cmds, cmd)
	return nil
}

// Cmds returns a shallow copy of tracked commands.
func (m *Manager) Cmds() []*exec.Cmd {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]*exec.Cmd, len(m.cmds))
	copy(out, m.cmds)
	return out
}

// Shutdown sends graceful signals then force-kills; always closes the Windows job
// so KILL_ON_JOB_CLOSE reaps the whole tree (including grandchildren).
func (m *Manager) Shutdown(ctx context.Context) {
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		return
	}
	cmds := append([]*exec.Cmd(nil), m.cmds...)
	m.mu.Unlock()

	for i := len(cmds) - 1; i >= 0; i-- {
		c := cmds[i]
		if c.Process == nil {
			continue
		}
		_ = terminate(c)
	}

	done := make(chan struct{})
	go func() {
		for _, c := range cmds {
			if c.Process != nil {
				_, _ = c.Process.Wait()
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-ctx.Done():
		for _, c := range cmds {
			if c.Process != nil {
				_ = killForce(c)
			}
		}
		<-done
	}

	m.mu.Lock()
	m.closeJob()
	m.closed = true
	m.mu.Unlock()
}

// Close immediately kills the job/group (safe to call multiple times; use as defer).
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return
	}
	for _, c := range m.cmds {
		if c.Process != nil {
			_ = killForce(c)
		}
	}
	m.closeJob()
	m.closed = true
}

// RunDetach is unused helper to document API.
func RunDetach(_ *exec.Cmd) error {
	return fmt.Errorf("detached processes are not supported; children must stay under ptdev")
}

var _ = os.Interrupt
