//go:build !windows

package cli

import (
	"os"
	"syscall"
)

func launchSignals() []os.Signal {
	return []os.Signal{os.Interrupt, syscall.SIGTERM, syscall.SIGHUP}
}
