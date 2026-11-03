package services

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pt-dev/ptdev/internal/config"
	"github.com/pt-dev/ptdev/internal/httpproxy"
	"github.com/pt-dev/ptdev/internal/logx"
	"github.com/pt-dev/ptdev/internal/modules"
	"github.com/pt-dev/ptdev/internal/proc"
	"github.com/pt-dev/ptdev/internal/runconf"
)

// Stack holds running services.
type Stack struct {
	Cfg   *config.Config
	Paths *modules.Paths
	Proc  *proc.Manager
	Proxy *httpproxy.Server
}

// Start boots database, Mailpit, PHP FastCGI, Apache according to config.
func (s *Stack) Start() error {
	if s.Cfg.MySQL.Enabled {
		if s.Cfg.MySQL.IsPostgres() {
			if err := s.startPostgres(); err != nil {
				return err
			}
		} else if err := s.startMySQL(); err != nil {
			return err
		}
	}
	if s.Cfg.Mailpit.Enabled && s.Paths.MailpitBin != "" {
		if err := s.startMailpit(); err != nil {
			return err
		}
	}
	if s.Cfg.PHP.Enabled && s.Cfg.PHP.FastCGIPort != 0 {
		if err := s.startPHPFastCGI(); err != nil {
			return err
		}
	}
	if s.Cfg.Apache.Enabled {
		if err := s.startApache(); err != nil {
			return err
		}
	}
	return nil
}

func (s *Stack) startPHPFastCGI() error {
	phpCGI := s.Paths.PHPBin
	cand := filepath.Join(filepath.Dir(s.Paths.PHPBin), "php-cgi")
	if _, err := os.Stat(cand); err == nil {
		phpCGI = cand
	}
	if _, err := os.Stat(cand + ".exe"); err == nil {
		phpCGI = cand + ".exe"
	}
	if p, err := modules.FindBinary(s.Paths.PHP, "php-cgi"); err == nil {
		phpCGI = p
	}

	addr := s.Cfg.PHP.FastCGIAddr()
	cgiDir := filepath.Join(s.Cfg.RuntimePath(), "cgi")
	phpDir := filepath.Dir(s.Paths.PHPBin)

	cmd := exec.Command(phpCGI, "-b", addr)
	cmd.Dir = phpDir
	cmd.Env = append(os.Environ(),
		"PHPRC="+cgiDir,
		"PHP_INI_SCAN_DIR=",
		"PHP_FCGI_MAX_REQUESTS=10000",
		"PHP_FCGI_CHILDREN=4",
		"PATH="+phpDir+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	logPath := filepath.Join(s.Cfg.LogsPath(), "php-fcgi.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	logx.Info("starting PHP FastCGI on %s …", addr)
	logx.Debug("php-cgi: %s", phpCGI)
	if err := s.Proc.Start(cmd); err != nil {
		return fmt.Errorf("start php-cgi: %w", err)
	}
	if cmd.Process != nil {
		logx.Verbose("php-cgi pid %d", cmd.Process.Pid)
	}
	host := s.Cfg.PHP.FastCGIHost
	if host == "" {
		host = "127.0.0.1"
	}
	if err := waitReady(cmd, host, s.Cfg.PHP.FastCGIPort, 20*time.Second, logPath); err != nil {
		return fmt.Errorf("PHP FastCGI not ready: %w", err)
	}
	logx.Success("PHP FastCGI ready")
	return nil
}

func (s *Stack) startMySQL() error {
	cnf := runconf.MyCNF(s.Cfg)
	basedir := s.Paths.MySQL
	label := s.Cfg.MySQL.DisplayName()
	cmd := exec.Command(s.Paths.MysqldBin,
		"--defaults-file="+cnf,
		"--basedir="+basedir,
		"--datadir="+s.Cfg.DBDataPath(),
		"--port="+fmt.Sprintf("%d", s.Cfg.MySQL.Port),
	)
	cmd.Dir = filepath.Dir(s.Paths.MysqldBin)
	logPath := filepath.Join(s.Cfg.LogsPath(), "mysql-stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	logx.Info("starting %s on :%d …", label, s.Cfg.MySQL.Port)
	logx.Debug("mysqld: %s", s.Paths.MysqldBin)
	logx.Debug("my.cnf: %s", cnf)
	logx.Debug("datadir: %s", s.Cfg.DBDataPath())
	if err := s.Proc.Start(cmd); err != nil {
		return fmt.Errorf("start mysqld: %w", err)
	}
	if cmd.Process != nil {
		logx.Verbose("mysqld pid %d", cmd.Process.Pid)
	}
	if err := waitReady(cmd, "127.0.0.1", s.Cfg.MySQL.Port, 60*time.Second, logPath, filepath.Join(s.Cfg.LogsPath(), "mysql.log")); err != nil {
		return fmt.Errorf("%s not ready: %w", label, err)
	}
	logx.Success("%s ready", label)
	return nil
}

func (s *Stack) startPostgres() error {
	if s.Paths.PostgresBin == "" {
		return fmt.Errorf("postgres binary not found")
	}
	data := s.Cfg.DBDataPath()
	binDir := filepath.Dir(s.Paths.PostgresBin)
	cmd := exec.Command(s.Paths.PostgresBin,
		"-D", data,
		"-p", fmt.Sprintf("%d", s.Cfg.MySQL.Port),
		"-h", "127.0.0.1",
		"-c", "logging_collector=off",
		"-c", "log_destination=stderr",
	)
	cmd.Dir = binDir
	cmd.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	logPath := filepath.Join(s.Cfg.LogsPath(), "postgres-stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	logx.Info("starting PostgreSQL on :%d …", s.Cfg.MySQL.Port)
	logx.Debug("postgres: %s", s.Paths.PostgresBin)
	logx.Debug("datadir: %s", data)
	if err := s.Proc.Start(cmd); err != nil {
		return fmt.Errorf("start postgres: %w", err)
	}
	if cmd.Process != nil {
		logx.Verbose("postgres pid %d", cmd.Process.Pid)
	}
	if err := waitReady(cmd, "127.0.0.1", s.Cfg.MySQL.Port, 60*time.Second, logPath); err != nil {
		return fmt.Errorf("postgres not ready: %w", err)
	}
	logx.Success("PostgreSQL ready")
	return nil
}

func (s *Stack) startApache() error {
	conf := runconf.HTTPDConf(s.Cfg)
	serverRoot := runconf.ApacheServerRoot(s.Paths)
	binDir := filepath.Join(serverRoot, "bin")

	// Fail fast on config errors (e.g. missing .so) instead of waiting for TCP timeout.
	test := exec.Command(s.Paths.HTTPDBin, "-f", conf, "-t")
	test.Dir = serverRoot
	test.Env = append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if out, err := test.CombinedOutput(); err != nil {
		msg := strings.TrimSpace(string(out))
		if msg == "" {
			msg = err.Error()
		}
		return fmt.Errorf("apache config invalid:\n%s", msg)
	}
	logx.Debug("httpd -t ok")
	logx.Debug("httpd.conf: %s", conf)
	logx.Debug("PHPRC: %s", filepath.Join(s.Cfg.RuntimePath(), "cgi"))

	cmd := exec.Command(s.Paths.HTTPDBin, "-f", conf, "-D", "FOREGROUND")
	cmd.Dir = serverRoot
	env := os.Environ()
	if s.Cfg.PHP.Enabled {
		cgiDir := filepath.Join(s.Cfg.RuntimePath(), "cgi")
		phpDir := filepath.Dir(s.Paths.PHPBin)
		env = append(env,
			"PHPRC="+cgiDir,
			"PHP_INI_SCAN_DIR=",
			"PATH="+phpDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		)
	}
	env = append(env, "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	cmd.Env = env
	logPath := filepath.Join(s.Cfg.LogsPath(), "apache-stdout.log")
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err == nil {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}
	logx.Info("starting Apache on :%d …", s.Cfg.Apache.Port)
	logx.Debug("httpd: %s", s.Paths.HTTPDBin)
	logx.Debug("httpd listen: %s", s.Cfg.Apache.HTTPDListen())
	if err := s.Proc.Start(cmd); err != nil {
		return fmt.Errorf("start httpd: %w", err)
	}
	if cmd.Process != nil {
		logx.Verbose("httpd pid %d", cmd.Process.Pid)
	}
	backendPort := s.Cfg.Apache.BackendPort()
	if err := waitReady(cmd, "127.0.0.1", backendPort, 30*time.Second, logPath, filepath.Join(s.Cfg.LogsPath(), "apache.log")); err != nil {
		return fmt.Errorf("apache not ready: %w", err)
	}

	// Debug only: reverse proxy on the public port logs request/response bodies.
	if s.Cfg.Apache.BindPort != 0 {
		backend := fmt.Sprintf("http://127.0.0.1:%d", s.Cfg.Apache.BindPort)
		public := fmt.Sprintf(":%d", s.Cfg.Apache.Port)
		px, err := httpproxy.Start(public, backend)
		if err != nil {
			return fmt.Errorf("http log proxy: %w", err)
		}
		s.Proxy = px
		logx.Info("request body proxy on :%d (httpd is on 127.0.0.1:%d)", s.Cfg.Apache.Port, s.Cfg.Apache.BindPort)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf("http://127.0.0.1:%d/", s.Cfg.Apache.Port), nil)
	resp, err := http.DefaultClient.Do(req)
	if err == nil {
		_ = resp.Body.Close()
	}
	logx.Success("Apache ready")
	for _, ip := range lanIPv4() {
		logx.Info("  lan: http://%s:%d/", ip, s.Cfg.Apache.Port)
	}
	return nil
}

// StopHTTPLogProxy shuts down the optional request-logging reverse proxy.
func (s *Stack) StopHTTPLogProxy() {
	if s.Proxy == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = s.Proxy.Shutdown(ctx)
	s.Proxy = nil
}

func (s *Stack) startMailpit() error {
	cmd := exec.Command(s.Paths.MailpitBin,
		"--smtp", fmt.Sprintf("127.0.0.1:%d", s.Cfg.Mailpit.SMTPPort),
		"--listen", fmt.Sprintf("127.0.0.1:%d", s.Cfg.Mailpit.UIPort),
	)
	logx.Info("starting Mailpit SMTP :%d UI :%d …", s.Cfg.Mailpit.SMTPPort, s.Cfg.Mailpit.UIPort)
	if err := s.Proc.Start(cmd); err != nil {
		return fmt.Errorf("start mailpit: %w", err)
	}
	_ = waitTCP("127.0.0.1", s.Cfg.Mailpit.UIPort, 15*time.Second)
	logx.Success("Mailpit ready")
	return nil
}

func waitTCP(host string, port int, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		time.Sleep(250 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s", addr)
}

// lanIPv4 returns non-loopback IPv4 addresses on up interfaces (LAN / Wi‑Fi / etc.).
func lanIPv4() []string {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, a := range addrs {
			var ip net.IP
			switch v := a.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ip == nil || ip.IsLoopback() {
				continue
			}
			ip4 := ip.To4()
			if ip4 == nil || ip4.IsLinkLocalUnicast() {
				continue
			}
			s := ip4.String()
			if seen[s] {
				continue
			}
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// waitReady waits for TCP listen; on timeout prints log tails and notes if the process already exited.
func waitReady(cmd *exec.Cmd, host string, port int, timeout time.Duration, logPaths ...string) error {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(host, fmt.Sprintf("%d", port))
	for time.Now().Before(deadline) {
		if processExited(cmd) {
			tailLogs(logPaths...)
			return fmt.Errorf("process exited before ready (see logs above); target %s", addr)
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			logx.Debug("%s is accepting connections", addr)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	if processExited(cmd) {
		tailLogs(logPaths...)
		return fmt.Errorf("process exited before ready (see logs above); target %s", addr)
	}
	tailLogs(logPaths...)
	return fmt.Errorf("timeout waiting for %s (see logs above)", addr)
}

func processExited(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return true
	}
	if cmd.ProcessState != nil && cmd.ProcessState.Exited() {
		return true
	}
	return !procAlive(cmd.Process.Pid)
}

func tailLogs(paths ...string) {
	for _, p := range paths {
		data, err := os.ReadFile(p)
		if err != nil || len(data) == 0 {
			continue
		}
		if len(data) > 4000 {
			data = data[len(data)-4000:]
		}
		logx.Error("---- %s (tail) ----\n%s", filepath.Base(p), string(data))
	}
}

// SetupHTTPS runs mkcert for local certs when enabled.
func SetupHTTPS(cfg *config.Config, paths *modules.Paths) error {
	if !cfg.Apache.HTTPS || paths.MkcertBin == "" {
		return nil
	}
	cert := filepath.Join(cfg.RuntimePath(), "cert.pem")
	key := filepath.Join(cfg.RuntimePath(), "key.pem")
	if _, err := os.Stat(cert); err == nil {
		return nil
	}
	host := "localhost"
	if cfg.PrettyURL != "" {
		host = cfg.PrettyURL
	}
	logx.Info("generating TLS cert with mkcert …")
	_ = exec.Command(paths.MkcertBin, "-install").Run()
	cmd := exec.Command(paths.MkcertBin, "-cert-file", cert, "-key-file", key, host, "localhost", "127.0.0.1")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkcert: %v\n%s", err, out)
	}
	logx.Success("TLS cert ready")
	return nil
}
