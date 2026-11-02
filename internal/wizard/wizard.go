package wizard

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pt-dev/ptdev/internal/config"
	"github.com/pt-dev/ptdev/internal/envfile"
	"github.com/pt-dev/ptdev/internal/logx"
	"github.com/pt-dev/ptdev/internal/modules"
	"github.com/pt-dev/ptdev/internal/ports"
)

// RunInteractive creates a new project via prompts.
func RunInteractive() error {
	in := bufio.NewReader(os.Stdin)
	ask := func(label, def string) string {
		if def != "" {
			fmt.Printf("%s [%s]: ", label, def)
		} else {
			fmt.Printf("%s: ", label)
		}
		line, _ := in.ReadString('\n')
		line = strings.TrimSpace(line)
		if line == "" {
			return def
		}
		return line
	}
	askBool := func(label string, def bool) bool {
		d := "n"
		if def {
			d = "y"
		}
		v := strings.ToLower(ask(label+" (y/n)", d))
		return v == "y" || v == "yes"
	}
	askInt := func(label string, def int) int {
		v := ask(label, strconv.Itoa(def))
		n, err := strconv.Atoi(v)
		if err != nil {
			return def
		}
		return n
	}

	fmt.Println("PT.DEV — create a new project")
	name := ask("Project name", "myapp")
	cwd, _ := os.Getwd()
	location := ask("Location", filepath.Join(cwd, name))
	docroot := ask("Document root (relative to project)", "public")

	phpOn := askBool("Enable PHP", true)
	phpVer := "8.3"
	xdebug := false
	if phpOn {
		cat, err := modules.LoadCatalog()
		vers := []string{"8.1", "8.2", "8.3", "8.4"}
		if err == nil {
			vers = cat.PHPVersions()
		}
		phpVer = ask(fmt.Sprintf("PHP version (%s)", strings.Join(vers, "/")), "8.3")
		xdebug = askBool("Enable Xdebug", false)
	}

	apacheOn := askBool("Enable Apache", true)
	apachePort := 8080
	https := false
	if apacheOn {
		apachePort = askInt("Apache HTTP port", 8080)
		if suggested, msg := ports.SuggestIfBusy("127.0.0.1", apachePort); msg != "" {
			logx.Warn("%s", msg)
			if askBool(fmt.Sprintf("Use port %d instead", suggested), true) {
				apachePort = suggested
			}
		}
		https = askBool("Enable HTTPS (mkcert)", false)
	}

	dbOn := askBool("Enable database", true)
	engine := config.EngineMariaDB
	dbPort := 3306
	dbUser := "root"
	dbPass := ""
	dbName := name
	if dbOn {
		engine = strings.ToLower(ask("Database engine (mariadb/mysql/postgres)", "mariadb"))
		switch engine {
		case "mysql", "oracle", "mysql-community":
			engine = config.EngineMySQL
		case "postgres", "postgresql", "pgsql", "pg":
			engine = config.EnginePostgres
		default:
			engine = config.EngineMariaDB
		}
		defPort := 3306
		defUser := "root"
		if engine == config.EnginePostgres {
			defPort = 5432
			defUser = "postgres"
		}
		label := "MariaDB"
		switch engine {
		case config.EngineMySQL:
			label = "MySQL"
		case config.EnginePostgres:
			label = "PostgreSQL"
		}
		dbPort = askInt(label+" port", defPort)
		suggested, msg := ports.SuggestIfBusy("127.0.0.1", dbPort)
		if msg != "" {
			logx.Warn("%s", msg)
			if askBool(fmt.Sprintf("Use port %d instead", suggested), true) {
				dbPort = suggested
			}
		}
		dbUser = ask("Database username", defUser)
		dbPass = ask("Database password (empty = none)", "")
		dbName = ask("Database name", name)
	}

	phpInfoOn := askBool("Enable /.info (phpinfo)", true)
	phpMyAdminOn := askBool("Enable /phpmyadmin/", true)

	mailpit := askBool("Enable Mailpit (dev mail catcher)", false)
	pretty := ask("Pretty URL (e.g. myapp.test, empty to skip)", "")

	cfg := config.Default(name, location)
	cfg.Docroot = docroot
	cfg.PHP.Enabled = phpOn
	cfg.PHP.Version = phpVer
	cfg.PHP.Xdebug = xdebug
	cfg.Apache.Enabled = apacheOn
	cfg.Apache.Port = apachePort
	cfg.Apache.HTTPS = https
	cfg.MySQL.Enabled = dbOn
	cfg.MySQL.Engine = engine
	cfg.MySQL.Port = dbPort
	cfg.MySQL.User = dbUser
	cfg.MySQL.Password = dbPass
	cfg.MySQL.Database = dbName
	cfg.Mailpit.Enabled = mailpit
	cfg.PrettyURL = pretty
	cfg.PHPInfo = config.BoolPtr(phpInfoOn)
	cfg.PHPMyAdmin = config.BoolPtr(phpMyAdminOn)

	fmt.Println()
	fmt.Printf("Create project %q at %s ?\n", name, location)
	if !askBool("Confirm", true) {
		logx.Warn("cancelled")
		return nil
	}

	if err := os.MkdirAll(location, 0o755); err != nil {
		return err
	}
	cfg.Location = location
	if err := cfg.EnsureScaffold(); err != nil {
		return err
	}
	if err := cfg.Save(); err != nil {
		return err
	}
	if err := config.EnsureGitignore(location); err != nil {
		logx.Warn("gitignore: %v", err)
	}
	if err := envfile.WriteDBKeys(cfg); err != nil {
		logx.Warn(".env: %v", err)
	}

	logx.Success("project created: %s", location)
	logx.Info("launch with: ptdev launch %q", location)
	if phpInfoOn {
		logx.Info("phpinfo will be at /.info after launch")
	}
	if phpMyAdminOn {
		logx.Info("phpMyAdmin will be at /phpmyadmin/ after launch")
	}
	return nil
}
