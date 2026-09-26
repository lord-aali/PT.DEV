package ports

import (
	"fmt"
	"net"
)

// InUse reports whether something is listening on tcp host:port.
func InUse(host string, port int) bool {
	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", host, port))
	if err != nil {
		return true
	}
	_ = ln.Close()
	return false
}

// NextFree finds the next free TCP port starting at start (inclusive), checking host.
func NextFree(host string, start int) (int, error) {
	for p := start; p < start+1000; p++ {
		if !InUse(host, p) {
			return p, nil
		}
	}
	return 0, fmt.Errorf("no free port found near %d", start)
}

// SuggestIfBusy returns port if free, otherwise next free and a warning message.
func SuggestIfBusy(host string, port int) (int, string) {
	if !InUse(host, port) {
		return port, ""
	}
	next, err := NextFree(host, port+1)
	if err != nil {
		return port, fmt.Sprintf("port %d is in use", port)
	}
	return next, fmt.Sprintf("port %d is in use; suggesting %d", port, next)
}
