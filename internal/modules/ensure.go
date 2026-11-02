package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pt-dev/ptdev/internal/logx"
)

// Paths holds resolved module install paths for a project launch.
type Paths struct {
	PHP          string
	Apache       string
	MySQL        string // MariaDB or Oracle MySQL basedir
	Postgres     string
	PHPMyAdmin   string
	Composer     string
	Mailpit      string
	Mkcert       string
	XdebugDLL    string
	PHPBin       string
	HTTPDBin     string
	MysqldBin    string
	MysqlBin     string
	Mysqldump    string
	PostgresBin  string
	InitdbBin    string
	PsqlBin      string
	PgDumpBin    string
	MailpitBin   string
	MkcertBin    string
	ComposerPhar string
}

// EnsureForConfig downloads modules required by the project configuration.
// dbEngine is mariadb|mysql|postgres (empty treated as mariadb when dbEnabled).
// Incomplete caches are re-downloaded automatically.
func (c *Cache) EnsureForConfig(phpEnabled bool, phpVer string, apache, dbEnabled bool, dbEngine string, phpMyAdmin, mailpit, https, xdebug bool) (*Paths, error) {
	p := &Paths{}
	var err error

	if phpEnabled {
		p.PHP, err = c.Ensure("php", phpVer)
		if err != nil {
			return nil, err
		}
		p.PHPBin, err = FindBinary(p.PHP, "php")
		if err != nil {
			return nil, err
		}
		p.Composer, err = c.Ensure("composer", "")
		if err != nil {
			return nil, err
		}
		p.ComposerPhar = filepath.Join(p.Composer, "composer.phar")

		if xdebug {
			xd, xerr := c.Ensure("xdebug", phpVer)
			if xerr != nil {
				logx.Warn("xdebug: %v", xerr)
			} else {
				p.XdebugDLL = filepath.Join(xd, "php_xdebug.dll")
			}
		}
	}

	if apache {
		p.Apache, err = c.Ensure("apache", "")
		if err != nil {
			return nil, err
		}
		p.HTTPDBin, err = FindBinary(p.Apache, "httpd")
		if err != nil {
			p.HTTPDBin, err = FindBinary(p.Apache, "apache2")
			if err != nil {
				return nil, err
			}
		}
	}

	if dbEnabled {
		engine := strings.ToLower(strings.TrimSpace(dbEngine))
		if engine == "" {
			engine = "mariadb"
		}
		switch engine {
		case "postgres", "postgresql", "pgsql", "pg":
			if err := ensurePostgres(c, p); err != nil {
				return nil, err
			}
		case "mysql", "oracle", "mysql-community", "oracle-mysql":
			if err := ensureMySQLFamily(c, p, "mysql-community"); err != nil {
				return nil, err
			}
		default: // mariadb
			if err := ensureMySQLFamily(c, p, "mysql"); err != nil {
				return nil, err
			}
		}
	}

	if phpEnabled && dbEnabled && phpMyAdmin {
		engine := strings.ToLower(strings.TrimSpace(dbEngine))
		if engine == "" {
			engine = "mariadb"
		}
		// phpMyAdmin speaks MySQL protocol only.
		if engine != "postgres" && engine != "postgresql" && engine != "pgsql" && engine != "pg" {
			p.PHPMyAdmin, err = c.Ensure("phpmyadmin", "")
			if err != nil {
				return nil, err
			}
		}
	}

	if mailpit {
		p.Mailpit, err = c.Ensure("mailpit", "")
		if err != nil {
			return nil, err
		}
		p.MailpitBin, err = FindBinary(p.Mailpit, "mailpit")
		if err != nil {
			return nil, err
		}
	}

	if https {
		p.Mkcert, err = c.Ensure("mkcert", "")
		if err != nil {
			return nil, err
		}
		p.MkcertBin, err = FindBinary(p.Mkcert, "mkcert")
		if err != nil {
			return nil, err
		}
	}

	return p, nil
}

func ensureMySQLFamily(c *Cache, p *Paths, module string) error {
	var err error
	p.MySQL, err = c.Ensure(module, "")
	if err != nil {
		return err
	}
	p.MysqldBin, err = FindBinary(p.MySQL, "mysqld")
	if err != nil {
		p.MysqldBin, err = FindBinary(p.MySQL, "mariadbd")
		if err != nil {
			return fmt.Errorf("%s server binary: %w", module, err)
		}
	}
	p.MysqlBin, _ = FindBinary(p.MySQL, "mysql")
	if p.MysqlBin == "" {
		p.MysqlBin, _ = FindBinary(p.MySQL, "mariadb")
	}
	p.Mysqldump, _ = FindBinary(p.MySQL, "mysqldump")
	if p.Mysqldump == "" {
		p.Mysqldump, _ = FindBinary(p.MySQL, "mariadb-dump")
	}
	return nil
}

func ensurePostgres(c *Cache, p *Paths) error {
	var err error
	p.Postgres, err = c.Ensure("postgres", "")
	if err != nil {
		return err
	}
	p.PostgresBin, err = FindBinary(p.Postgres, "postgres")
	if err != nil {
		return fmt.Errorf("postgres binary: %w", err)
	}
	p.InitdbBin, err = FindBinary(p.Postgres, "initdb")
	if err != nil {
		return fmt.Errorf("initdb binary: %w", err)
	}
	p.PsqlBin, _ = FindBinary(p.Postgres, "psql")
	p.PgDumpBin, _ = FindBinary(p.Postgres, "pg_dump")
	return nil
}

// CacheSize returns approximate size of the modules cache in bytes.
func (c *Cache) CacheSize() int64 {
	var total int64
	_ = filepath.Walk(c.Root, func(_ string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		total += info.Size()
		return nil
	})
	return total
}
