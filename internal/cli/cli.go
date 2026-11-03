package cli

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/pt-dev/ptdev/internal/browser"
	"github.com/pt-dev/ptdev/internal/config"
	"github.com/pt-dev/ptdev/internal/db"
	"github.com/pt-dev/ptdev/internal/envfile"
	"github.com/pt-dev/ptdev/internal/hosts"
	"github.com/pt-dev/ptdev/internal/lock"
	"github.com/pt-dev/ptdev/internal/logx"
	"github.com/pt-dev/ptdev/internal/modules"
	"github.com/pt-dev/ptdev/internal/ports"
	"github.com/pt-dev/ptdev/internal/proc"
	"github.com/pt-dev/ptdev/internal/runconf"
	"github.com/pt-dev/ptdev/internal/services"
	"github.com/pt-dev/ptdev/internal/wizard"
)

var (
	logLevel string
	noColor  bool
	version  string
)

// Execute runs the root command.
func Execute(ver string) error {
	version = ver
	normalizeLogFlag(os.Args)
	root := &cobra.Command{
		Use:           "ptdev",
		Short:         "PT.DEV — portable local PHP/Apache/database environments",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       ver,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			logx.SetNoColor(noColor)
			if err := logx.SetLevel(logLevel); err != nil {
				return err
			}
			if logx.IsVerbose() {
				logx.Verbose("log level: %s", logx.LevelName())
			}
			return nil
		},
	}
	root.PersistentFlags().StringVar(&logLevel, "log", "", "log level: verbose (proxy: method+URL) | debug (+ headers/bodies, error logs)")
	root.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable colorized output")

	root.AddCommand(
		cmdVersion(),
		cmdNew(),
		cmdLaunch(),
		cmdCache(),
		cmdStatus(),
		cmdLogs(),
		cmdShell(),
	)

	if err := root.Execute(); err != nil {
		logx.Error("%v", err)
		return err
	}
	return nil
}

func cmdVersion() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show PT.DEV version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("ptdev %s (%s/%s)\n", version, runtime.GOOS, runtime.GOARCH)
		},
	}
}

func cmdNew() *cobra.Command {
	return &cobra.Command{
		Use:   "new",
		Short: "Interactively create a new project",
		RunE: func(cmd *cobra.Command, args []string) error {
			return wizard.RunInteractive()
		},
	}
}

func cmdLaunch() *cobra.Command {
	var openBrowser bool
	c := &cobra.Command{
		Use:   "launch [projectPath]",
		Short: "Launch the stack for a project",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			return runLaunch(path, openBrowser)
		},
	}
	c.Flags().BoolVar(&openBrowser, "browser", true, "open site in browser when ready")
	return c
}

func runLaunch(projectPath string, openBrowser bool) error {
	cfg, err := config.Load(projectPath)
	if err != nil {
		return err
	}
	logx.Verbose("project: %s (%s)", cfg.Name, cfg.Location)
	logx.Debug("config: %s", cfg.Path())
	logx.Debug("docroot: %s", cfg.DocrootAbs())
	logx.Debug("runtime: %s", cfg.RuntimePath())
	logx.Debug("sessions: %s", cfg.SessionsPath())

	release, err := lock.Acquire(cfg)
	if err != nil {
		return err
	}
	defer release()
	logx.Debug("launch lock acquired: %s", cfg.LockPath())

	// Port checks
	if cfg.Apache.Enabled {
		if suggested, msg := ports.SuggestIfBusy("127.0.0.1", cfg.Apache.Port); msg != "" {
			logx.Warn("%s", msg)
			cfg.Apache.Port = suggested
		} else {
			logx.Debug("apache port %d free", cfg.Apache.Port)
		}
	}
	if cfg.MySQL.Enabled {
		if suggested, msg := ports.SuggestIfBusy("127.0.0.1", cfg.MySQL.Port); msg != "" {
			logx.Warn("%s", msg)
			cfg.MySQL.Port = suggested
		} else {
			logx.Debug("%s port %d free", cfg.MySQL.DisplayName(), cfg.MySQL.Port)
		}
	}

	if cfg.PrettyURL != "" {
		if err := hosts.Ensure(cfg.PrettyURL); err != nil {
			logx.Warn("pretty URL hosts entry: %v", err)
		} else {
			logx.Success("hosts: %s → 127.0.0.1", cfg.PrettyURL)
		}
	}

	cache, err := modules.NewCache()
	if err != nil {
		return err
	}
	logx.Verbose("module cache: %s", cache.Root)

	// Bodies aren't in Apache's access log. With -log debug only, bind httpd on a
	// loopback port and put a logging reverse proxy on the public port.
	if cfg.Apache.Enabled && logx.IsDebug() {
		backend, err := ports.NextFree("127.0.0.1", cfg.Apache.Port+10000)
		if err != nil {
			return fmt.Errorf("allocate httpd backend port: %w", err)
		}
		cfg.Apache.BindHost = "127.0.0.1"
		cfg.Apache.BindPort = backend
		logx.Verbose("HTTP body proxy will listen on :%d → httpd 127.0.0.1:%d", cfg.Apache.Port, backend)
	}

	// Persistent php-cgi FastCGI (avoids classic CGI spawn-per-request slowness).
	if cfg.PHP.Enabled && cfg.Apache.Enabled {
		fcgiPort, err := ports.NextFree("127.0.0.1", 9000)
		if err != nil {
			return fmt.Errorf("allocate PHP FastCGI port: %w", err)
		}
		cfg.PHP.FastCGIHost = "127.0.0.1"
		cfg.PHP.FastCGIPort = fcgiPort
		logx.Verbose("PHP FastCGI will listen on 127.0.0.1:%d", fcgiPort)
	}

	paths, err := cache.EnsureForConfig(
		cfg.PHP.Enabled, cfg.PHP.Version,
		cfg.Apache.Enabled, cfg.MySQL.Enabled, cfg.MySQL.NormalizedEngine(),
		cfg.PHPMyAdminEnabled(),
		cfg.Mailpit.Enabled, cfg.Apache.HTTPS, cfg.PHP.Xdebug,
	)
	if err != nil {
		return err
	}
	logx.Debug("php bin: %s", paths.PHPBin)
	logx.Debug("httpd: %s", paths.HTTPDBin)
	if cfg.MySQL.IsPostgres() {
		logx.Debug("postgres: %s", paths.PostgresBin)
	} else {
		logx.Debug("mysqld: %s", paths.MysqldBin)
	}
	logx.Debug("phpmyadmin: %s", paths.PHPMyAdmin)

	if err := runconf.Generate(cfg, paths); err != nil {
		return err
	}
	logx.Verbose("generated runtime configs under %s", cfg.RuntimePath())
	_ = envfile.WriteDBKeys(cfg)

	if err := services.SetupHTTPS(cfg, paths); err != nil {
		logx.Warn("https: %v", err)
	}

	pm, err := proc.New()
	if err != nil {
		return err
	}
	// Kill children first, then wipe ephemeral datadir (Windows locks files while the server runs).
	defer func() {
		pm.Close()
		if cfg.MySQL.Enabled {
			if err := db.ClearDataDir(cfg); err != nil {
				logx.Warn("%v", err)
			}
		}
	}()

	stack := &services.Stack{Cfg: cfg, Paths: paths, Proc: pm}
	defer stack.StopHTTPLogProxy()

	if cfg.MySQL.Enabled {
		// datadir is ephemeral: always rebuild from database.sql so a hard-killed
		// previous run cannot leave a corrupt datadir that times out on listen.
		if err := db.ClearDataDir(cfg); err != nil {
			logx.Warn("%v", err)
		}
		if err := db.PrepareDataDir(cfg, paths); err != nil {
			return err
		}
	}

	if err := stack.Start(); err != nil {
		return err
	}

	if cfg.MySQL.Enabled {
		if err := db.ImportSQL(cfg, paths); err != nil {
			logx.Warn("import: %v", err)
		}
	}

	siteURL := fmt.Sprintf("http://127.0.0.1:%d/", cfg.Apache.Port)
	if cfg.PrettyURL != "" {
		scheme := "http"
		if cfg.Apache.HTTPS {
			scheme = "https"
		}
		siteURL = fmt.Sprintf("%s://%s/", scheme, cfg.PrettyURL)
	}
	logx.Success("ready — %s", siteURL)
	if cfg.PHP.Enabled && cfg.PHPInfoEnabled() {
		logx.Info("phpinfo: %s.info", siteURL)
	}
	if cfg.PHP.Enabled && cfg.MySQL.Enabled && cfg.MySQL.IsMySQLFamily() && cfg.PHPMyAdminEnabled() {
		logx.Info("phpMyAdmin (auto-login): %sphpmyadmin/", siteURL)
	}
	if cfg.Mailpit.Enabled {
		logx.Info("Mailpit UI: http://127.0.0.1:%d/", cfg.Mailpit.UIPort)
	}
	logx.Info("press Ctrl+C to stop (all child processes will terminate)")

	if openBrowser && cfg.Apache.Enabled {
		_ = browser.Open(siteURL)
	}

	ctx, stop := signal.NotifyContext(context.Background(), launchSignals()...)
	defer stop()

	followCtx, cancelFollow := context.WithCancel(ctx)
	defer cancelFollow()
	// Verbose+: Apache Common access log → [http] METHOD path status.
	// Debug: also tails error logs; body peeks come from the reverse proxy.
	if cfg.Apache.Enabled && logx.IsVerbose() {
		logx.FollowAccessLogs(followCtx, cfg.LogsPath())
	}

	<-ctx.Done()
	logx.Info("shutting down …")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if cfg.MySQL.Enabled {
		if err := db.DumpSQL(cfg, paths); err != nil {
			logx.Warn("dump sql: %v", err)
		} else {
			logx.Success("database.sql updated")
		}
	}
	pm.Shutdown(shutdownCtx)
	logx.Success("stopped")
	return nil
}

func cmdStatus() *cobra.Command {
	return &cobra.Command{
		Use:   "status [projectPath]",
		Short: "Show project and module cache status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			cache, err := modules.NewCache()
			if err != nil {
				return err
			}
			fmt.Printf("Project:  %s\n", cfg.Name)
			fmt.Printf("Location: %s\n", cfg.Location)
			fmt.Printf("Docroot:  %s\n", cfg.Docroot)
			fmt.Printf("PHP:      %v %s (xdebug=%v)\n", cfg.PHP.Enabled, cfg.PHP.Version, cfg.PHP.Xdebug)
			fmt.Printf("Apache:   %v :%d https=%v\n", cfg.Apache.Enabled, cfg.Apache.Port, cfg.Apache.HTTPS)
			fmt.Printf("Database: %v %s :%d %s@%s\n", cfg.MySQL.Enabled, cfg.MySQL.DisplayName(), cfg.MySQL.Port, cfg.MySQL.User, cfg.MySQL.Database)
			fmt.Printf("Mailpit:  %v\n", cfg.Mailpit.Enabled)
			fmt.Printf("Pretty:   %s\n", cfg.PrettyURL)
			fmt.Printf("Cache:    %s (%s)\n", cache.Root, logx.FormatBytes(cache.CacheSize()))
			if _, err := os.Stat(cfg.LockPath()); err == nil {
				fmt.Println("Launch:   locked (running?)")
			} else {
				fmt.Println("Launch:   not running")
			}
			return nil
		},
	}
}

func cmdLogs() *cobra.Command {
	var follow bool
	c := &cobra.Command{
		Use:   "logs [projectPath]",
		Short: "Show project logs",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			dir := cfg.LogsPath()
			entries, err := os.ReadDir(dir)
			if err != nil {
				return err
			}
			if !follow {
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					p := filepath.Join(dir, e.Name())
					fmt.Printf("===== %s =====\n", e.Name())
					data, err := os.ReadFile(p)
					if err != nil {
						continue
					}
					if len(data) > 32*1024 {
						data = data[len(data)-32*1024:]
					}
					fmt.Println(string(data))
				}
				return nil
			}
			// Simple follow of all log files
			logx.Info("following %s (Ctrl+C to stop)", dir)
			ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
			defer stop()
			offsets := map[string]int64{}
			for {
				select {
				case <-ctx.Done():
					return nil
				default:
				}
				for _, e := range entries {
					if e.IsDir() {
						continue
					}
					p := filepath.Join(dir, e.Name())
					f, err := os.Open(p)
					if err != nil {
						continue
					}
					off := offsets[p]
					st, _ := f.Stat()
					if st.Size() < off {
						off = 0
					}
					_, _ = f.Seek(off, io.SeekStart)
					buf, _ := io.ReadAll(f)
					_ = f.Close()
					if len(buf) > 0 {
						fmt.Printf("[%s] %s", e.Name(), string(buf))
						offsets[p] = off + int64(len(buf))
					}
				}
				time.Sleep(500 * time.Millisecond)
				entries, _ = os.ReadDir(dir)
			}
		},
	}
	c.Flags().BoolVarP(&follow, "follow", "f", false, "follow log output")
	return c
}

func cmdShell() *cobra.Command {
	return &cobra.Command{
		Use:   "shell [projectPath]",
		Short: "Open a shell with php/db clients/composer on PATH",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "."
			if len(args) > 0 {
				path = args[0]
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			cache, err := modules.NewCache()
			if err != nil {
				return err
			}
			paths, err := cache.EnsureForConfig(
				cfg.PHP.Enabled, cfg.PHP.Version,
				false, cfg.MySQL.Enabled, cfg.MySQL.NormalizedEngine(),
				false, // phpMyAdmin not needed for shell
				false, false, false,
			)
			if err != nil {
				return err
			}

			pathParts := []string{}
			if paths.PHPBin != "" {
				pathParts = append(pathParts, filepath.Dir(paths.PHPBin))
			}
			if paths.MysqlBin != "" {
				pathParts = append(pathParts, filepath.Dir(paths.MysqlBin))
			}
			if paths.PsqlBin != "" {
				pathParts = append(pathParts, filepath.Dir(paths.PsqlBin))
			}
			if paths.Composer != "" {
				pathParts = append(pathParts, paths.Composer)
				// helper composer.bat/sh
				if err := writeComposerWrapper(paths.Composer, paths.PHPBin, paths.ComposerPhar); err != nil {
					logx.Warn("composer wrapper: %v", err)
				}
			}
			newPath := strings.Join(pathParts, string(os.PathListSeparator)) + string(os.PathListSeparator) + os.Getenv("PATH")

			var shell *exec.Cmd
			if runtime.GOOS == "windows" {
				shell = exec.Command("cmd.exe")
			} else {
				sh := os.Getenv("SHELL")
				if sh == "" {
					sh = "/bin/bash"
				}
				shell = exec.Command(sh)
			}
			shell.Dir = cfg.Location
			shell.Stdin = os.Stdin
			shell.Stdout = os.Stdout
			shell.Stderr = os.Stderr
			shell.Env = append(os.Environ(),
				"PATH="+newPath,
				"PHPRC="+cfg.RuntimePath(),
			)
			logx.Info("shell ready (php/db clients/composer on PATH). type exit to leave.")
			return shell.Run()
		},
	}
}

func writeComposerWrapper(dir, phpBin, phar string) error {
	if runtime.GOOS == "windows" {
		content := fmt.Sprintf("@echo off\r\n\"%s\" \"%s\" %%*\r\n", phpBin, phar)
		return os.WriteFile(filepath.Join(dir, "composer.bat"), []byte(content), 0o644)
	}
	content := fmt.Sprintf("#!/bin/sh\nexec \"%s\" \"%s\" \"$@\"\n", phpBin, phar)
	return os.WriteFile(filepath.Join(dir, "composer"), []byte(content), 0o755)
}

// normalizeLogFlag rewrites GNU-style -log / -invalidate into -- forms for pflag.
func normalizeLogFlag(args []string) {
	for i := 1; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "-log":
			args[i] = "--log"
		case strings.HasPrefix(a, "-log="):
			args[i] = "--log=" + strings.TrimPrefix(a, "-log=")
		case a == "-invalidate":
			args[i] = "--invalidate"
		case a == "-invalidate-caches": // legacy alias
			args[i] = "--invalidate"
		}
	}
}
