# PT.DEV

Portable local PHP / Apache / database development environments.

Supported databases: **MariaDB** (default), **Oracle MySQL**, and **PostgreSQL**.

## Build

```bash
go build -o ptdev.exe ./cmd/ptdev
```

## Usage

```text
ptdev [-log verbose|debug] [--no-color] version
ptdev [-log verbose|debug] new
ptdev [-log verbose|debug] launch [path] [--browser]
ptdev [-log verbose|debug] cache [--host|--all] [--invalidate]
ptdev status [path]
ptdev logs [-f] [path]
ptdev shell [path]
```

`-log verbose` prints extra launch steps and tails Apache’s **Common** access log as `→ [http] METHOD path status`.  
`-log debug` adds request/response headers and body peeks via a reverse proxy (Apache access logs cannot capture bodies), plus Apache/PHP error log tails.

PHP runs through a **persistent FastCGI** `php-cgi` process (Apache `mod_proxy_fcgi`), not classic CGI. That keeps responses close to XAMPP-speed; OPcache is enabled when the DLL/SO is present.

### Module catalog

On `cache` / `launch` / `new`, ptdev **prefers a remote catalog** over the copy embedded in the binary:

```text
https://raw.githubusercontent.com/lord-aali/PT.DEV/refs/heads/master/internal/modules/catalog.yaml
```

If the remote fetch fails (offline, 404, timeout), it falls back to the embedded `internal/modules/catalog.yaml`.

**Forks:** if you move or rename the catalog, either:

1. Change `DefaultCatalogURL` in `internal/modules/catalog.go` before building, **or**
2. Set `PTDEV_CATALOG_URL` to your raw catalog URL at runtime (no rebuild needed):

```bash
# Windows PowerShell
$env:PTDEV_CATALOG_URL = "https://raw.githubusercontent.com/YOUR_USER/YOUR_FORK/refs/heads/master/internal/modules/catalog.yaml"

# Unix
export PTDEV_CATALOG_URL="https://raw.githubusercontent.com/YOUR_USER/YOUR_FORK/refs/heads/master/internal/modules/catalog.yaml"
```

Use `-log verbose` to see which source was used (`catalog: remote …` vs `catalog: embedded`).

### Module cache

```text
ptdev cache                    # ask: this host only, or all platforms
ptdev cache --host             # this machine only
ptdev cache --all              # every OS/arch in the catalog
ptdev cache --invalidate       # wipe modules/ (asks for confirmation), then cache
ptdev cache -invalidate --all  # same (GNU-style flag), then re-download all
```

`--invalidate` / `-invalidate` permanently deletes the `modules/` directory next to the binary after you type `yes`, then continues with the normal cache flow (`--host`, `--all`, or interactive).

Modules are stored next to the binary under `modules/`. Incomplete or empty module folders are detected on `launch` / `cache` and re-downloaded automatically.

Project config lives in `{project}/.pt.dev/config.yaml`. Database dumps are stored in `.pt.dev/database.sql`.

### Database engines

In `ptdev new`, choose `mariadb`, `mysql`, or `postgres`. You can also set this in config:

```yaml
mysql:
  enabled: true
  engine: mariadb   # mariadb | mysql | postgres
  port: 3306        # 5432 for postgres
  user: root        # postgres for postgres
  password: ""
  database: myapp
```

| Engine | What ships | Default port / user | Admin UI |
|--------|------------|---------------------|----------|
| `mariadb` | MariaDB 11.4 (catalog key `mysql`) | 3306 / `root` | phpMyAdmin |
| `mysql` | Oracle MySQL Community 8.0 | 3306 / `root` | phpMyAdmin |
| `postgres` | PostgreSQL 16 (theseus-rs binaries) | 5432 / `postgres` | — |

Ephemeral data lives under `.pt.dev/mysql-data/` or `.pt.dev/postgres-data/` and is rebuilt from `database.sql` on each launch. `.env` gets `DB_CONNECTION=mysql` or `pgsql` accordingly.

### Dev endpoints

`ptdev new` asks whether to enable these (both default **y**). Missing keys in older configs stay enabled.

```yaml
phpinfo: true       # /.info → phpinfo(); set false to disable
phpmyadmin: true    # /phpmyadmin/; set false to disable (MariaDB/MySQL only)
```

### URLs (after `launch`)

| Path | What |
|------|------|
| `/` | Document root (Apache directory listing when no `index.php` / `index.html`) |
| `/.info` | `phpinfo()` (when `phpinfo: true`) |
| `/phpmyadmin/` | phpMyAdmin auto-login (when `phpmyadmin: true` and MariaDB/MySQL) |

Example: `http://127.0.0.1:8080/.info`

### Stopping

All child processes (Apache, database, Mailpit, …) terminate when `ptdev` exits — including if you close the CLI window (on Windows they are reaped via a Job Object). Prefer **Ctrl+C**: that path dumps `database.sql` and clears the ephemeral datadir before exit. Closing the window still kills the children, but usually skips the dump/clear (the next `launch` rebuilds the datadir from `database.sql`).
