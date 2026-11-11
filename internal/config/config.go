package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	DirName      = ".pt.dev"
	ConfigFile   = "config.yaml"
	DatabaseSQL  = "database.sql"
	MySQLData    = "mysql-data"
	PostgresData = "postgres-data"
	RuntimeDir   = "runtime"
	LogsDir      = "logs"
	TmpDir       = "tmp"
	LockFile     = "launch.lock"

	EngineMariaDB  = "mariadb"
	EngineMySQL    = "mysql" // Oracle MySQL Community
	EnginePostgres = "postgres"
)

// Config is the per-project PT.DEV configuration.
type Config struct {
	Version   int     `yaml:"version"`
	Name      string  `yaml:"name"`
	Docroot   string  `yaml:"docroot"`
	PHP       PHP     `yaml:"php"`
	Apache    Apache  `yaml:"apache"`
	MySQL     MySQL   `yaml:"mysql"`
	Mailpit   Mailpit `yaml:"mailpit"`
	PrettyURL string  `yaml:"pretty_url"`

	// Location is the project root on disk. Not saved in config.yaml — set from the
	// path passed to `ptdev launch` / `Load` (or the wizard create path).
	Location string `yaml:"-"`

	// PHPInfo / PHPMyAdmin use pointers so a missing YAML key defaults to enabled.
	PHPInfo    *bool `yaml:"phpinfo"`
	PHPMyAdmin *bool `yaml:"phpmyadmin"`
}

// BoolPtr returns a *bool for YAML defaults / wizard values.
func BoolPtr(v bool) *bool { return &v }

// PHPInfoEnabled reports whether /.info is exposed (default true when unset).
func (c *Config) PHPInfoEnabled() bool {
	if c.PHPInfo == nil {
		return true
	}
	return *c.PHPInfo
}

// PHPMyAdminEnabled reports whether /phpmyadmin/ is exposed (default true when unset).
func (c *Config) PHPMyAdminEnabled() bool {
	if c.PHPMyAdmin == nil {
		return true
	}
	return *c.PHPMyAdmin
}

type PHP struct {
	Enabled bool   `yaml:"enabled"`
	Version string `yaml:"version"`
	Xdebug  bool   `yaml:"xdebug"`

	// Runtime-only: persistent php-cgi FastCGI listener (proxy:fcgi).
	FastCGIHost string `yaml:"-"`
	FastCGIPort int    `yaml:"-"`
}

// FastCGIAddr is host:port for Apache proxy:fcgi, or empty when unset.
func (p PHP) FastCGIAddr() string {
	if p.FastCGIPort == 0 {
		return ""
	}
	host := p.FastCGIHost
	if host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("%s:%d", host, p.FastCGIPort)
}

type Apache struct {
	Enabled bool `yaml:"enabled"`
	Port    int  `yaml:"port"`
	HTTPS   bool `yaml:"https"`

	// Runtime-only (not saved): when BindPort != 0, httpd listens on BindHost:BindPort
	// and an HTTP log proxy occupies Port.
	BindHost string `yaml:"-"`
	BindPort int    `yaml:"-"`
}

// HTTPDListen returns the Listen directive value for httpd.conf.
func (a Apache) HTTPDListen() string {
	port := a.Port
	if a.BindPort != 0 {
		port = a.BindPort
	}
	if a.BindHost != "" {
		return fmt.Sprintf("%s:%d", a.BindHost, port)
	}
	return fmt.Sprintf("%d", port)
}

// BackendPort is where httpd actually listens.
func (a Apache) BackendPort() int {
	if a.BindPort != 0 {
		return a.BindPort
	}
	return a.Port
}

// MySQL holds database settings (yaml key kept as "mysql" for backward compatibility).
// Engine selects MariaDB, Oracle MySQL, or PostgreSQL.
type MySQL struct {
	Enabled  bool   `yaml:"enabled"`
	Engine   string `yaml:"engine"` // mariadb | mysql | postgres (empty = mariadb)
	Port     int    `yaml:"port"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
	Database string `yaml:"database"`
}

// NormalizedEngine returns mariadb, mysql, or postgres.
func (m MySQL) NormalizedEngine() string {
	switch strings.ToLower(strings.TrimSpace(m.Engine)) {
	case "", "mariadb", "maria":
		return EngineMariaDB
	case "mysql", "oracle", "mysql-community", "oracle-mysql":
		return EngineMySQL
	case "postgres", "postgresql", "pgsql", "pg":
		return EnginePostgres
	default:
		return strings.ToLower(strings.TrimSpace(m.Engine))
	}
}

// IsPostgres reports whether the configured engine is PostgreSQL.
func (m MySQL) IsPostgres() bool {
	return m.NormalizedEngine() == EnginePostgres
}

// IsMySQLFamily reports MariaDB or Oracle MySQL.
func (m MySQL) IsMySQLFamily() bool {
	e := m.NormalizedEngine()
	return e == EngineMariaDB || e == EngineMySQL
}

// DefaultPort returns the usual listen port for the engine.
func (m MySQL) DefaultPort() int {
	if m.IsPostgres() {
		return 5432
	}
	return 3306
}

// DefaultUser returns the usual superuser name for the engine.
func (m MySQL) DefaultUser() string {
	if m.IsPostgres() {
		return "postgres"
	}
	return "root"
}

// ConnectionDriver is the Laravel/PHP-style DB_CONNECTION value.
func (m MySQL) ConnectionDriver() string {
	if m.IsPostgres() {
		return "pgsql"
	}
	return "mysql"
}

// DisplayName is a short label for logs/UI.
func (m MySQL) DisplayName() string {
	switch m.NormalizedEngine() {
	case EngineMySQL:
		return "MySQL"
	case EnginePostgres:
		return "PostgreSQL"
	default:
		return "MariaDB"
	}
}

type Mailpit struct {
	Enabled  bool `yaml:"enabled"`
	SMTPPort int  `yaml:"smtp_port"`
	UIPort   int  `yaml:"ui_port"`
}

// Default returns a sensible empty config.
func Default(name, location string) *Config {
	return &Config{
		Version:  1,
		Name:     name,
		Location: location,
		Docroot:  "public",
		PHP: PHP{
			Enabled: true,
			Version: "8.3",
			Xdebug:  false,
		},
		Apache: Apache{
			Enabled: true,
			Port:    8080,
			HTTPS:   false,
		},
		MySQL: MySQL{
			Enabled:  true,
			Engine:   EngineMariaDB,
			Port:     3306,
			User:     "root",
			Password: "",
			Database: name,
		},
		Mailpit: Mailpit{
			Enabled:  false,
			SMTPPort: 1025,
			UIPort:   8025,
		},
		PHPInfo:    BoolPtr(true),
		PHPMyAdmin: BoolPtr(true),
	}
}

// Dir returns {location}/.pt.dev
func (c *Config) Dir() string {
	return filepath.Join(c.Location, DirName)
}

// Path returns the config.yaml path.
func (c *Config) Path() string {
	return filepath.Join(c.Dir(), ConfigFile)
}

// DocrootAbs returns absolute document root.
// "/", ".", and "" mean the project root (wizard uses "/" for that).
func (c *Config) DocrootAbs() string {
	d := strings.TrimSpace(c.Docroot)
	if d == "" || d == "." || d == "/" || d == `\` {
		return c.Location
	}
	if filepath.IsAbs(d) {
		return d
	}
	return filepath.Join(c.Location, d)
}

// DatabaseSQLPath returns path to portable dump.
func (c *Config) DatabaseSQLPath() string {
	return filepath.Join(c.Dir(), DatabaseSQL)
}

// MySQLDataPath returns the ephemeral MariaDB/MySQL datadir (legacy name).
func (c *Config) MySQLDataPath() string {
	return filepath.Join(c.Dir(), MySQLData)
}

// DBDataPath returns the ephemeral datadir for the configured engine.
func (c *Config) DBDataPath() string {
	if c.MySQL.IsPostgres() {
		return filepath.Join(c.Dir(), PostgresData)
	}
	return filepath.Join(c.Dir(), MySQLData)
}

// RuntimePath returns generated configs dir.
func (c *Config) RuntimePath() string {
	return filepath.Join(c.Dir(), RuntimeDir)
}

// LogsPath returns logs directory.
func (c *Config) LogsPath() string {
	return filepath.Join(c.Dir(), LogsDir)
}

// TmpPath returns ephemeral temp directory under .pt.dev.
func (c *Config) TmpPath() string {
	return filepath.Join(c.Dir(), TmpDir)
}

// SessionsPath is PHP session.save_path.
func (c *Config) SessionsPath() string {
	return filepath.Join(c.TmpPath(), "sessions")
}

// UploadTmpPath is PHP upload_tmp_dir / sys_temp_dir.
func (c *Config) UploadTmpPath() string {
	return filepath.Join(c.TmpPath(), "uploads")
}

// PMATempPath is phpMyAdmin TempDir.
func (c *Config) PMATempPath() string {
	return filepath.Join(c.TmpPath(), "phpmyadmin")
}

// LockPath returns launch lock path.
func (c *Config) LockPath() string {
	return filepath.Join(c.Dir(), LockFile)
}

// Save writes config.yaml.
func (c *Config) Save() error {
	if err := os.MkdirAll(c.Dir(), 0o755); err != nil {
		return err
	}
	data, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.Path(), data, 0o644)
}

// Load reads config from a project root (directory containing .pt.dev or the .pt.dev itself).
func Load(projectPath string) (*Config, error) {
	root, err := ResolveProjectRoot(projectPath)
	if err != nil {
		return nil, err
	}
	path := filepath.Join(root, DirName, ConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	var c Config
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	c.Location = root
	if c.MySQL.Enabled && strings.TrimSpace(c.MySQL.Engine) == "" {
		c.MySQL.Engine = EngineMariaDB
	}
	return &c, nil
}

// ResolveProjectRoot finds the project directory from a path.
func ResolveProjectRoot(p string) (string, error) {
	abs, err := filepath.Abs(p)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		abs = filepath.Dir(abs)
	}
	// If pointing at .pt.dev, go up one.
	if filepath.Base(abs) == DirName {
		return filepath.Dir(abs), nil
	}
	cfg := filepath.Join(abs, DirName, ConfigFile)
	if _, err := os.Stat(cfg); err == nil {
		return abs, nil
	}
	return "", fmt.Errorf("no %s/%s found under %s — run: ptdev new", DirName, ConfigFile, abs)
}

// EnsureScaffold creates dirs and empty database.sql.
func (c *Config) EnsureScaffold() error {
	dirs := []string{c.Dir(), c.RuntimePath(), c.LogsPath(), c.TmpPath(), c.SessionsPath(), c.UploadTmpPath(), c.PMATempPath(), c.DocrootAbs()}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	sqlPath := c.DatabaseSQLPath()
	if _, err := os.Stat(sqlPath); os.IsNotExist(err) {
		header := fmt.Sprintf("-- PT.DEV portable dump for %s\n", c.Name)
		if err := os.WriteFile(sqlPath, []byte(header), 0o644); err != nil {
			return err
		}
	}
	return nil
}
