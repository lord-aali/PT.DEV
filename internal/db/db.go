package db

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/pt-dev/ptdev/internal/config"
	"github.com/pt-dev/ptdev/internal/logx"
	"github.com/pt-dev/ptdev/internal/modules"
)

// ClearDataDir removes the ephemeral database datadir (portable dump is database.sql).
func ClearDataDir(cfg *config.Config) error {
	data := cfg.DBDataPath()
	if _, err := os.Stat(data); os.IsNotExist(err) {
		return nil
	}
	if err := os.RemoveAll(data); err != nil {
		return fmt.Errorf("clear %s: %w", filepath.Base(data), err)
	}
	logx.Info("cleared %s (state kept in database.sql)", filepath.Base(data))
	return nil
}

// PrepareDataDir initializes the engine datadir if empty or broken.
func PrepareDataDir(cfg *config.Config, paths *modules.Paths) error {
	if cfg.MySQL.IsPostgres() {
		return preparePostgres(cfg, paths)
	}
	return prepareMySQL(cfg, paths)
}

func mysqlDatadirLooksValid(data string) bool {
	candidates := []string{
		filepath.Join(data, "mysql", "db.MAD"),
		filepath.Join(data, "mysql", "db.frm"),
		filepath.Join(data, "mysql", "db.ibd"),
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return true
		}
	}
	return false
}

func postgresDatadirLooksValid(data string) bool {
	_, err := os.Stat(filepath.Join(data, "PG_VERSION"))
	return err == nil
}

func prepareMySQL(cfg *config.Config, paths *modules.Paths) error {
	data := cfg.DBDataPath()
	if err := os.MkdirAll(data, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(data)
	if err != nil {
		return err
	}
	label := cfg.MySQL.DisplayName()
	if len(entries) > 0 && mysqlDatadirLooksValid(data) {
		logx.Info("%s datadir ready", label)
		return nil
	}
	if len(entries) > 0 && !mysqlDatadirLooksValid(data) {
		logx.Warn("%s datadir incomplete/corrupt — re-initializing", label)
		if err := os.RemoveAll(data); err != nil {
			return err
		}
		if err := os.MkdirAll(data, 0o755); err != nil {
			return err
		}
	}

	logx.Info("initializing %s datadir …", label)
	if err := initializeMySQLDataDir(data, paths); err != nil {
		return err
	}
	if !mysqlDatadirLooksValid(data) {
		return fmt.Errorf("datadir init finished but system tables are missing under %s", data)
	}
	logx.Success("%s datadir initialized", label)
	return nil
}

func preparePostgres(cfg *config.Config, paths *modules.Paths) error {
	data := cfg.DBDataPath()
	if err := os.MkdirAll(filepath.Dir(data), 0o755); err != nil {
		return err
	}
	if st, err := os.Stat(data); err == nil && st.IsDir() {
		entries, _ := os.ReadDir(data)
		if len(entries) > 0 && postgresDatadirLooksValid(data) {
			logx.Info("PostgreSQL datadir ready")
			return nil
		}
		if len(entries) > 0 {
			logx.Warn("PostgreSQL datadir incomplete/corrupt — re-initializing")
			if err := os.RemoveAll(data); err != nil {
				return err
			}
		}
	}

	if paths.InitdbBin == "" {
		return fmt.Errorf("initdb not found")
	}
	user := cfg.MySQL.User
	if user == "" {
		user = "postgres"
	}
	logx.Info("initializing PostgreSQL datadir …")
	// Localhost-only stack: trust auth keeps empty-password Laravel/PHP apps working.
	args := []string{
		"-D", data,
		"-U", user,
		"--encoding=UTF8",
		"--locale=C",
		"--auth-local=trust",
		"--auth-host=trust",
	}
	cmd := exec.Command(paths.InitdbBin, args...)
	cmd.Dir = filepath.Dir(paths.InitdbBin)
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(paths.InitdbBin)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("initdb: %v\n%s", err, out)
	}
	if err := writePostgresHBA(data); err != nil {
		return err
	}
	if !postgresDatadirLooksValid(data) {
		return fmt.Errorf("initdb finished but PG_VERSION missing under %s", data)
	}
	logx.Success("PostgreSQL datadir initialized")
	return nil
}

func writePostgresHBA(data string) error {
	hba := `# PT.DEV — localhost trust (dev only)
local   all             all                                     trust
host    all             all             127.0.0.1/32            trust
host    all             all             ::1/128                 trust
`
	return os.WriteFile(filepath.Join(data, "pg_hba.conf"), []byte(hba), 0o600)
}

// initializeMySQLDataDir bootstraps a fresh MariaDB/MySQL datadir.
// MariaDB Windows does NOT support mysqld --initialize-insecure (unknown option;
// it still creates partial InnoDB files then aborts — the source of prior timeouts).
func initializeMySQLDataDir(data string, paths *modules.Paths) error {
	basedir := paths.MySQL

	for _, name := range []string{"mariadb-install-db", "mysql_install_db"} {
		bin, err := modules.FindBinary(paths.MySQL, name)
		if err != nil {
			continue
		}
		args := []string{"--datadir=" + data, "--password="}
		cmd := exec.Command(bin, args...)
		cmd.Dir = filepath.Dir(bin)
		out, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("%s: %v\n%s", name, err, out)
		}
		return nil
	}

	cmd := exec.Command(paths.MysqldBin,
		"--datadir="+data,
		"--basedir="+basedir,
		"--initialize-insecure",
	)
	cmd.Dir = filepath.Dir(paths.MysqldBin)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mysqld --initialize-insecure: %v\n%s", err, out)
	}
	return nil
}

// ImportSQL loads database.sql into the running server.
func ImportSQL(cfg *config.Config, paths *modules.Paths) error {
	if cfg.MySQL.IsPostgres() {
		return importPostgres(cfg, paths)
	}
	return importMySQL(cfg, paths)
}

func sqlBodyHasStatements(data []byte) bool {
	body := strings.TrimSpace(string(data))
	for _, line := range strings.Split(body, "\n") {
		t := strings.TrimSpace(line)
		if t == "" || strings.HasPrefix(t, "--") {
			continue
		}
		return true
	}
	return false
}

func importMySQL(cfg *config.Config, paths *modules.Paths) error {
	sqlPath := cfg.DatabaseSQLPath()
	data, err := os.ReadFile(sqlPath)
	if err != nil {
		return err
	}

	mysql := paths.MysqlBin
	if mysql == "" {
		return fmt.Errorf("mysql client not found")
	}
	args := []string{
		"-h", "127.0.0.1",
		"-P", fmt.Sprintf("%d", cfg.MySQL.Port),
		"-u", cfg.MySQL.User,
	}
	if cfg.MySQL.Password != "" {
		args = append(args, "-p"+cfg.MySQL.Password)
	}

	create := fmt.Sprintf("CREATE DATABASE IF NOT EXISTS `%s`;", cfg.MySQL.Database)
	cmd := exec.Command(mysql, append(args, "-e", create)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("create database: %v\n%s", err, out)
	}

	if !sqlBodyHasStatements(data) {
		logx.Info("database.sql empty — fresh database %s", cfg.MySQL.Database)
		return nil
	}

	logx.Info("importing database.sql …")
	cmd = exec.Command(mysql, append(args, cfg.MySQL.Database)...)
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("import sql: %v\n%s", err, out)
	}
	logx.Success("database imported")
	return nil
}

func importPostgres(cfg *config.Config, paths *modules.Paths) error {
	if paths.PsqlBin == "" {
		return fmt.Errorf("psql not found")
	}
	sqlPath := cfg.DatabaseSQLPath()
	data, err := os.ReadFile(sqlPath)
	if err != nil {
		return err
	}

	env := pgEnv(cfg, paths)
	args := []string{
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", cfg.MySQL.Port),
		"-U", cfg.MySQL.User,
		"-d", "postgres",
		"-v", "ON_ERROR_STOP=1",
	}

	create := fmt.Sprintf("CREATE DATABASE %s;", pgIdent(cfg.MySQL.Database))
	cmd := exec.Command(paths.PsqlBin, append(args, "-c", create)...)
	cmd.Env = env
	if out, err := cmd.CombinedOutput(); err != nil {
		msg := string(out) + err.Error()
		if !strings.Contains(msg, "already exists") {
			return fmt.Errorf("create database: %v\n%s", err, out)
		}
	}

	if cfg.MySQL.Password != "" {
		setPass := fmt.Sprintf("ALTER USER %s WITH PASSWORD '%s';", pgIdent(cfg.MySQL.User), escapeSQLString(cfg.MySQL.Password))
		cmd = exec.Command(paths.PsqlBin, append(args, "-c", setPass)...)
		cmd.Env = env
		if out, err := cmd.CombinedOutput(); err != nil {
			logx.Warn("set postgres password: %v\n%s", err, out)
		}
	}

	if !sqlBodyHasStatements(data) {
		logx.Info("database.sql empty — fresh database %s", cfg.MySQL.Database)
		return nil
	}

	logx.Info("importing database.sql …")
	argsDB := []string{
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", cfg.MySQL.Port),
		"-U", cfg.MySQL.User,
		"-d", cfg.MySQL.Database,
		"-v", "ON_ERROR_STOP=1",
	}
	cmd = exec.Command(paths.PsqlBin, argsDB...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(data))
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("import sql: %v\n%s", err, out)
	}
	logx.Success("database imported")
	return nil
}

func pgEnv(cfg *config.Config, paths *modules.Paths) []string {
	binDir := ""
	if paths.PsqlBin != "" {
		binDir = filepath.Dir(paths.PsqlBin)
	} else if paths.PostgresBin != "" {
		binDir = filepath.Dir(paths.PostgresBin)
	}
	env := append(os.Environ(), "PATH="+binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	if cfg.MySQL.Password != "" {
		env = append(env, "PGPASSWORD="+cfg.MySQL.Password)
	}
	return env
}

func pgIdent(name string) string {
	// Quote if not a simple identifier.
	if name == "" {
		return `""`
	}
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			continue
		}
		return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
	}
	return name
}

func escapeSQLString(s string) string {
	return strings.ReplaceAll(s, `'`, `''`)
}

// DumpSQL writes the current database to database.sql.
func DumpSQL(cfg *config.Config, paths *modules.Paths) error {
	if cfg.MySQL.IsPostgres() {
		return dumpPostgres(cfg, paths)
	}
	return dumpMySQL(cfg, paths)
}

func dumpMySQL(cfg *config.Config, paths *modules.Paths) error {
	if paths.Mysqldump == "" {
		return fmt.Errorf("mysqldump not found")
	}
	args := []string{
		"-h", "127.0.0.1",
		"-P", fmt.Sprintf("%d", cfg.MySQL.Port),
		"-u", cfg.MySQL.User,
		"--databases", cfg.MySQL.Database,
		"--routines",
		"--triggers",
		"--single-transaction",
	}
	if cfg.MySQL.Password != "" {
		args = append(args, "-p"+cfg.MySQL.Password)
	}
	cmd := exec.Command(paths.Mysqldump, args...)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("mysqldump: %v\n%s", err, ee.Stderr)
		}
		return err
	}
	header := fmt.Sprintf("-- PT.DEV dump %s at %s\n", cfg.Name, time.Now().Format(time.RFC3339))
	return os.WriteFile(cfg.DatabaseSQLPath(), append([]byte(header), out...), 0o644)
}

func dumpPostgres(cfg *config.Config, paths *modules.Paths) error {
	if paths.PgDumpBin == "" {
		return fmt.Errorf("pg_dump not found")
	}
	args := []string{
		"-h", "127.0.0.1",
		"-p", fmt.Sprintf("%d", cfg.MySQL.Port),
		"-U", cfg.MySQL.User,
		"-d", cfg.MySQL.Database,
		"--no-owner",
		"--clean",
		"--if-exists",
	}
	cmd := exec.Command(paths.PgDumpBin, args...)
	cmd.Env = pgEnv(cfg, paths)
	out, err := cmd.Output()
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("pg_dump: %v\n%s", err, ee.Stderr)
		}
		return err
	}
	header := fmt.Sprintf("-- PT.DEV dump %s at %s\n", cfg.Name, time.Now().Format(time.RFC3339))
	return os.WriteFile(cfg.DatabaseSQLPath(), append([]byte(header), out...), 0o644)
}

// RecoverIfNeeded logs when a leftover datadir looks broken; PrepareDataDir re-inits.
func RecoverIfNeeded(cfg *config.Config, paths *modules.Paths) error {
	_ = paths
	data := cfg.DBDataPath()
	entries, err := os.ReadDir(data)
	if err != nil || len(entries) == 0 {
		return nil
	}
	valid := mysqlDatadirLooksValid(data)
	if cfg.MySQL.IsPostgres() {
		valid = postgresDatadirLooksValid(data)
	}
	if !valid {
		logx.Warn("%s present but invalid — will re-initialize", filepath.Base(data))
		return nil
	}
	logx.Info("%s present — crash-safe recover mode", filepath.Base(data))
	return nil
}
