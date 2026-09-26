package modules

import (
	"embed"
	"fmt"
	"runtime"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed catalog.yaml
var catalogFS embed.FS

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
	MySQL          simpleModule    `yaml:"mysql"`            // MariaDB
	MySQLCommunity simpleModule    `yaml:"mysql-community"` // Oracle MySQL
	Postgres       simpleModule    `yaml:"postgres"`
	Mailpit        simpleModule    `yaml:"mailpit"`
	Mkcert         simpleModule    `yaml:"mkcert"`
	Xdebug         versionedModule `yaml:"xdebug"`
}

// LoadCatalog parses the embedded catalog.
func LoadCatalog() (*Catalog, error) {
	data, err := catalogFS.ReadFile("catalog.yaml")
	if err != nil {
		return nil, err
	}
	var c Catalog
	if err := yaml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse catalog: %w", err)
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
