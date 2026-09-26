package lock

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/pt-dev/ptdev/internal/config"
)

// Acquire creates an exclusive launch.lock. Returns a release function.
func Acquire(cfg *config.Config) (func(), error) {
	path := cfg.LockPath()
	if data, err := os.ReadFile(path); err == nil {
		pid, _ := strconv.Atoi(string(data))
		if pid > 0 && processAlive(pid) {
			return nil, fmt.Errorf("project already launched (pid %d); stop the other ptdev first", pid)
		}
		_ = os.Remove(path)
	}

	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
	_, _ = fmt.Fprintf(f, "%d", os.Getpid())
	_ = f.Close()

	release := func() {
		_ = os.Remove(path)
	}
	return release, nil
}

func processAlive(pid int) bool {
	p, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return signalZero(p)
}

// staleAfter is unused helper reserved for future lock TTL.
var _ = time.Second
