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
ptdev [-log verbose|debug] cache [--host|--all] [--invalidate-caches]
ptdev status [path]
ptdev logs [-f] [path]
ptdev shell [path]
```

`-log verbose` prints extra launch steps and tails Apache’s **Common** access log as `→ [http] METHOD path status`.  
`-log debug` adds request/response headers and body peeks via a reverse proxy (Apache access logs cannot capture bodies), plus Apache/PHP error log tails.

### Module cache

```text
ptdev cache                           # ask: this host only, or all platforms
ptdev cache --host                    # this machine only
ptdev cache --all                     # every OS/arch in the catalog
ptdev cache --invalidate-caches       # wipe modules/ (asks for confirmation), then cache
ptdev cache -invalidate-caches --all  # same (GNU-style flag), then re-download all
```

`--invalidate-caches` / `-invalidate-caches` permanently deletes the `modules/` directory next to the binary after you type `yes`, then continues with the normal cache flow (`--host`, `--all`, or interactive).

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

### URLs (after `launch`)

| Path | What |
|------|------|
| `/` | Document root (Apache directory listing when no `index.php` / `index.html`) |
| `/.info` | `phpinfo()` |
| `/phpmyadmin/` | phpMyAdmin (auto-login when MariaDB/MySQL is enabled) |

Example: `http://127.0.0.1:8080/.info`

### Stopping

All child processes (Apache, database, Mailpit, …) terminate when `ptdev` exits — including if you close the CLI window (on Windows they are reaped via a Job Object). Prefer **Ctrl+C**: that path dumps `database.sql` and clears the ephemeral datadir before exit. Closing the window still kills the children, but usually skips the dump/clear (the next `launch` rebuilds the datadir from `database.sql`).
