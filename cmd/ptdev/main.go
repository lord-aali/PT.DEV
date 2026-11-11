package main

import (
	"os"

	"github.com/pt-dev/ptdev/internal/cli"
)

// Version is set via -ldflags at build time.
var Version = "0.3.1"

func main() {
	if err := cli.Execute(Version); err != nil {
		os.Exit(1)
	}
}
