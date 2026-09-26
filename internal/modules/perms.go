package modules

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

var (
	rePHPCLI = regexp.MustCompile(`^php\d+\.\d+$`)
	rePHPCGI = regexp.MustCompile(`^php-cgi\d+\.\d+$`)
	rePHPFPM = regexp.MustCompile(`^php-fpm\d+\.\d+$`)
)

// ensureUnixExec adds execute bits if missing (no-op on Windows).
func ensureUnixExec(path string) error {
	if runtime.GOOS == "windows" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return nil
	}
	mode := info.Mode()
	if mode&0o100 != 0 {
		return nil
	}
	return os.Chmod(path, mode|0o755)
}

// ensureModuleExecutables normalizes layouts and sets +x on Unix.
func ensureModuleExecutables(name, dest string) {
	if strings.EqualFold(name, "php") {
		normalizePHPLayout(dest)
	}
	if runtime.GOOS == "windows" {
		return
	}
	names := moduleExecNames(name)
	if len(names) == 0 {
		_ = filepath.WalkDir(dest, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			base := d.Name()
			if strings.Contains(base, ".") && !strings.HasPrefix(base, ".") {
				ext := strings.ToLower(filepath.Ext(base))
				switch ext {
				case ".so", ".a", ".o", ".h", ".c", ".php", ".ini", ".conf", ".txt", ".md", ".json", ".xml", ".html", ".css", ".js", ".map", ".dll", ".lib", ".pdb", ".phar":
					return nil
				}
			}
			_ = ensureUnixExec(path)
			return nil
		})
		return
	}

	for _, n := range names {
		if p, err := FindBinary(dest, n); err == nil {
			_ = ensureUnixExec(p)
		}
		direct := filepath.Join(dest, n)
		if st, err := os.Stat(direct); err == nil && !st.IsDir() {
			_ = ensureUnixExec(direct)
		}
	}
}

func moduleExecNames(name string) []string {
	switch strings.ToLower(name) {
	case "php":
		return []string{"php", "php-cgi", "php-fpm", "phpdbg"}
	case "apache":
		return []string{"httpd", "apache2", "apachectl", "htpasswd", "rotatelogs"}
	case "mysql", "mariadb", "mysql-community":
		return []string{"mysqld", "mariadbd", "mysql", "mariadb", "mysqldump", "mariadb-dump", "mysqladmin", "mariadb-admin", "mysql_install_db", "mariadb-install-db"}
	case "postgres", "postgresql":
		return []string{"postgres", "initdb", "psql", "pg_dump", "pg_restore", "pg_ctl", "createdb", "dropdb"}
	case "mailpit":
		return []string{"mailpit"}
	case "mkcert":
		return []string{"mkcert"}
	case "composer":
		return []string{"composer", "composer.phar"}
	default:
		return nil
	}
}

// normalizePHPLayout creates unversioned php / php-cgi / php-fpm aliases for
// shivammathur/php-builder trees that only ship php8.1, php-cgi8.1, etc.
func normalizePHPLayout(dest string) {
	cli := findNamedFile(dest, rePHPCLI)
	cgi := findNamedFile(dest, rePHPCGI)
	fpm := findNamedFile(dest, rePHPFPM)
	if cli != "" {
		ensureAliasBeside(cli, "php")
	}
	if cgi != "" {
		ensureAliasBeside(cgi, "php-cgi")
	}
	if fpm != "" {
		ensureAliasBeside(fpm, "php-fpm")
	}
}

func findNamedFile(root string, re *regexp.Regexp) string {
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || found != "" {
			return nil
		}
		if re.MatchString(d.Name()) {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found
}

func ensureAliasBeside(src, aliasName string) {
	alias := filepath.Join(filepath.Dir(src), aliasName)
	if st, err := os.Stat(alias); err == nil && !st.IsDir() {
		return
	}
	_ = os.Remove(alias)
	if err := os.Symlink(filepath.Base(src), alias); err == nil {
		_ = ensureUnixExec(alias)
		return
	}
	_ = copyFile(src, alias)
	_ = ensureUnixExec(alias)
}
