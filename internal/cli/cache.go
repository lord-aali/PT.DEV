package cli

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/pt-dev/ptdev/internal/logx"
	"github.com/pt-dev/ptdev/internal/modules"
)

func cmdCache() *cobra.Command {
	var all, host, invalidate bool
	c := &cobra.Command{
		Use:   "cache",
		Short: "Download and cache module binaries",
		Long: `Cache module binaries next to the ptdev executable.

  ptdev cache                  interactive: host only or all platforms
  ptdev cache --host           this machine only
  ptdev cache --all            every OS/arch listed in the catalog
  ptdev cache --invalidate     wipe modules/ (asks for confirmation), then cache

Catalog URLs are fetched from the remote catalog first (see README), with the
embedded catalog as fallback when offline.
`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if all && host {
				return fmt.Errorf("use either --host or --all, not both")
			}
			cache, err := modules.NewCache()
			if err != nil {
				return err
			}
			if invalidate {
				if err := confirmInvalidate(cache.Root); err != nil {
					return err
				}
				logx.Info("clearing module cache at %s …", cache.Root)
				if err := cache.InvalidateAll(); err != nil {
					return err
				}
				logx.Success("module cache cleared")
			}
			scope := ""
			switch {
			case all:
				scope = "all"
			case host:
				scope = "host"
			default:
				scope = askCacheScope()
			}
			return runCache(cache, scope)
		},
	}
	c.Flags().BoolVar(&all, "all", false, "cache binaries for all platforms in the catalog")
	c.Flags().BoolVar(&host, "host", false, "cache binaries for this host only")
	c.Flags().BoolVar(&invalidate, "invalidate", false, "delete all cached modules after confirmation")
	return c
}

func confirmInvalidate(root string) error {
	fmt.Printf("This will permanently delete the module cache at:\n  %s\nType 'yes' to confirm: ", root)
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(strings.ToLower(line))
	if line != "yes" {
		return fmt.Errorf("cancelled — cache not cleared")
	}
	return nil
}

func askCacheScope() string {
	t := modules.HostTarget()
	fmt.Println("PT.DEV — module cache")
	fmt.Printf("  1) This host only (%s)\n", t)
	fmt.Println("  2) All platforms in catalog")
	fmt.Print("Choose [1]: ")
	line, _ := bufio.NewReader(os.Stdin).ReadString('\n')
	line = strings.TrimSpace(line)
	if line == "2" {
		return "all"
	}
	return "host"
}

func runCache(cache *modules.Cache, scope string) error {
	var jobs []modules.CacheJob
	switch scope {
	case "all":
		jobs = cache.Catalog.AllCacheJobs()
		logx.Info("caching %d artifacts for all platforms …", len(jobs))
	default:
		jobs = cache.Catalog.HostCacheJobs()
		logx.Info("caching %d artifacts for %s …", len(jobs), modules.HostTarget())
	}
	ok, failed := cache.Prefetch(jobs)
	if failed > 0 {
		logx.Warn("finished with %d ok, %d failed", ok, failed)
		return fmt.Errorf("%d module(s) failed to cache", failed)
	}
	logx.Success("cached %d module(s) → %s (%s)", ok, cache.Root, logx.FormatBytes(cache.CacheSize()))
	return nil
}
