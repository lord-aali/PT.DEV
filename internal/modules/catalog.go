package modules

import (
	"embed"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/pt-dev/ptdev/internal/logx"
)

//go:embed catalog.yaml
var catalogFS embed.FS

// DefaultCatalogURL is the preferred remote catalog (master branch).
// Forks: change this constant when building, or set PTDEV_CATALOG_URL at runtime.
const DefaultCatalogURL = "https://raw.githubusercontent.com/lord-aali/PT.DEV/refs/heads/master/internal/modules/catalog.yaml"

// CatalogURLEnv is the env var forks/users can set to override DefaultCatalogURL.
const CatalogURLEnv = "PTDEV_CATALOG_URL"

// Artifact is a downloadable binary package.
type Artifact struct {
	OS              string `yaml:"os"`
	Arch            string `yaml:"arch"`
	URL             string `yaml:"url"`
	Filename        string `yaml:"filename"`
	Kind            string `yaml:"kind"` // zip, tar.gz, tar.xz, file
	Checksum        string `yaml:"checksum"`
	StripComponents int    `yaml:"strip_components"`
	Note            string `yaml:"note"`
}

type simpleModule struct {
	Version   string     `yaml:"version"`
	Artifacts []Artifact `yaml:"artifacts"`
}

type versionedModule struct {
	Versions map[string]struct {
		Artifacts []Artifact `yaml:"artifacts"`
	} `yaml:"versions"`
}

// Catalog holds all module metadata.
type Catalog struct {
	Composer       simpleModule    `yaml:"composer"`
	PHPMyAdmin     simpleModule    `yaml:"phpmyadmin"`
	PHP            versionedModule `yaml:"php"`
	Apache         simpleModule    `yaml:"apache"`
	MySQL          simpleModule    `yaml:"mysql"`           // MariaDB
	MySQLCommunity simpleModule    `yaml:"mysql-community"` // Oracle MySQL
	Postgres       simpleModule    `yaml:"postgres"`
	Mailpit        simpleModule    `yaml:"mailpit"`
	Mkcert         simpleModule    `yaml:"mkcert"`
	Xdebug         versionedModule `yaml:"xdebug"`

	// Source describes where the catalog was loaded from (remote URL or "embedded").
	Source string `yaml:"-"`
}

// CatalogURL returns the remote catalog URL (env override, else DefaultCatalogURL).
func CatalogURL() string {
	if u := strings.TrimSpace(os.Getenv(CatalogURLEnv)); u != "" {
		return u
	}
	return DefaultCatalogURL
}

// LoadCatalog prefers the remote catalog, then falls back to the embedded copy.
func LoadCatalog() (*Catalog, error) {
	url := CatalogURL()
	if c, err := loadCatalogRemote(url); err == nil {
		logx.Info("catalog fetched: %s", url)
		return c, nil
	} else {
		logx.Warn("catalog: remote unavailable (%v) — using embedded", err)
	}
	c, err := loadCatalogEmbedded()
	if err != nil {
		return nil, err
	}
	logx.Verbose("catalog: embedded")
	return c, nil
}

// LoadCatalogEmbedded returns only the binary-embedded catalog (no network).
func LoadCatalogEmbedded() (*Catalog, error) {
	return loadCatalogEmbedded()
}

func loadCatalogEmbedded() (*Catalog, error) {
	data, err := catalogFS.ReadFile("catalog.yaml")
	if err != nil {
		return nil, err
	}
	c, err := parseCatalog(data)
	if err != nil {
		return nil, err
	}
	c.Source = "embedded"
	return c, nil
}

func loadCatalogRemote(url string) (*Catalog, error) {
	client := &http.Client{Timeout: 15 * time.Second}
	if c := logx.Client(); c != nil {
		// Reuse transport for -log verbose/debug fetch logging, but keep a short timeout.
		cloned := *c
		cloned.Timeout = 15 * time.Second
		client = &cloned
	}
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ptdev")
	req.Header.Set("Accept", "text/yaml, text/plain, */*")
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20)) // 8 MiB cap
	if err != nil {
		return nil, err
	}
	c, err := parseCatalog(data)
	if err != nil {
		return nil, err
	}
	c.Source = url
	return c, nil
}

func parseCatalog(data []byte) (*Catalog, error) {
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
	}
	if len(c.PHP.Versions) == 0 && c.Apache.Version == "" && c.Composer.Version == "" {
		return nil, fmt.Errorf("parse catalog: empty or unrecognized catalog")
	}
	return &c, nil
}

// Platform returns goos/goarch normalized for catalog matching.
func Platform() (osName, arch string) {
	osName = runtime.GOOS
	arch = runtime.GOARCH
	if arch == "x86_64" {
		arch = "amd64"
	}
	return osName, arch
}

// PHPVersions lists catalogued PHP versions (sorted).
func (c *Catalog) PHPVersions() []string {
	out := make([]string, 0, len(c.PHP.Versions))
	for v := range c.PHP.Versions {
		out = append(out, v)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if out[j] < out[i] {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

func artifactsFor(c *Catalog, name, version string) (modVersion string, arts []Artifact, err error) {
	switch strings.ToLower(name) {
	case "composer":
		return c.Composer.Version, c.Composer.Artifacts, nil
	case "phpmyadmin":
		return c.PHPMyAdmin.Version, c.PHPMyAdmin.Artifacts, nil
	case "apache":
		return c.Apache.Version, c.Apache.Artifacts, nil
	case "mysql", "mariadb":
		return c.MySQL.Version, c.MySQL.Artifacts, nil
	case "mysql-community", "oracle-mysql":
		return c.MySQLCommunity.Version, c.MySQLCommunity.Artifacts, nil
	case "postgres", "postgresql":
		return c.Postgres.Version, c.Postgres.Artifacts, nil
	case "mailpit":
		return c.Mailpit.Version, c.Mailpit.Artifacts, nil
	case "mkcert":
		return c.Mkcert.Version, c.Mkcert.Artifacts, nil
	case "php":
		vset, ok := c.PHP.Versions[version]
		if !ok {
			return "", nil, fmt.Errorf("php version %s not in catalog", version)
		}
		return version, vset.Artifacts, nil
	case "xdebug":
		vset, ok := c.Xdebug.Versions[version]
		if !ok {
			return "", nil, fmt.Errorf("xdebug for php %s not in catalog", version)
		}
		return version, vset.Artifacts, nil
	default:
		return "", nil, fmt.Errorf("unknown module %q", name)
	}
}
