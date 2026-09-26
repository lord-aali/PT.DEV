package modules

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/pt-dev/ptdev/internal/logx"
)

// Target is an OS/arch pair for caching.
type Target struct {
	OS   string
	Arch string
}

func (t Target) String() string { return t.OS + "/" + t.Arch }

// HostTarget returns the current machine.
func HostTarget() Target {
	osName, arch := Platform()
	return Target{OS: osName, Arch: arch}
}

func matchArtifactFor(arts []Artifact, osName, arch string) (Artifact, error) {
	var fallback *Artifact
	for i := range arts {
		a := &arts[i]
		osOK := a.OS == "any" || a.OS == osName
		archOK := a.Arch == "any" || a.Arch == arch
		if osOK && archOK {
			return *a, nil
		}
		if a.OS == "any" && a.Arch == "any" && fallback == nil {
			fallback = a
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return Artifact{}, fmt.Errorf("no artifact for %s/%s", osName, arch)
}

// ResolveFor returns version + artifact for a specific platform.
func (c *Catalog) ResolveFor(name, version, osName, arch string) (string, Artifact, error) {
	modVer, arts, err := artifactsFor(c, name, version)
	if err != nil {
		return "", Artifact{}, err
	}
	art, err := matchArtifactFor(arts, osName, arch)
	return modVer, art, err
}

// Resolve uses the host platform.
func (c *Catalog) Resolve(name, version string) (string, Artifact, error) {
	osName, arch := Platform()
	return c.ResolveFor(name, version, osName, arch)
}

// CacheJob is one download unit.
type CacheJob struct {
	Name    string
	Version string
	Target  Target
}

// AllCacheJobs returns every catalog OS/arch artifact as a cache job.
func (c *Catalog) AllCacheJobs() []CacheJob {
	seen := map[string]bool{}
	var out []CacheJob
	addArts := func(name, ver string, arts []Artifact) {
		for _, a := range arts {
			t := Target{OS: a.OS, Arch: a.Arch}
			key := name + "|" + ver + "|" + t.OS + "|" + t.Arch + "|" + a.URL
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, CacheJob{Name: name, Version: ver, Target: t})
		}
	}
	addArts("composer", "", c.Composer.Artifacts)
	addArts("phpmyadmin", "", c.PHPMyAdmin.Artifacts)
	addArts("apache", "", c.Apache.Artifacts)
	addArts("mysql", "", c.MySQL.Artifacts)
	addArts("mysql-community", "", c.MySQLCommunity.Artifacts)
	addArts("postgres", "", c.Postgres.Artifacts)
	addArts("mailpit", "", c.Mailpit.Artifacts)
	addArts("mkcert", "", c.Mkcert.Artifacts)
	for ver, vset := range c.PHP.Versions {
		addArts("php", ver, vset.Artifacts)
	}
	for ver, vset := range c.Xdebug.Versions {
		addArts("xdebug", ver, vset.Artifacts)
	}
	return out
}

// HostCacheJobs returns jobs for the current host.
func (c *Catalog) HostCacheJobs() []CacheJob {
	t := HostTarget()
	var out []CacheJob
	for _, name := range []string{"composer", "phpmyadmin", "apache", "mysql", "mysql-community", "postgres", "mailpit", "mkcert"} {
		if _, _, err := c.ResolveFor(name, "", t.OS, t.Arch); err == nil {
			out = append(out, CacheJob{Name: name, Target: t})
		} else if _, _, err := c.ResolveFor(name, "", "any", "any"); err == nil {
			out = append(out, CacheJob{Name: name, Target: Target{OS: "any", Arch: "any"}})
		}
	}
	for _, ver := range c.PHPVersions() {
		if _, _, err := c.ResolveFor("php", ver, t.OS, t.Arch); err == nil {
			out = append(out, CacheJob{Name: "php", Version: ver, Target: t})
		}
		if _, _, err := c.ResolveFor("xdebug", ver, t.OS, t.Arch); err == nil {
			out = append(out, CacheJob{Name: "xdebug", Version: ver, Target: t})
		}
	}
	return out
}

// ModuleReady reports whether dest contains usable module files.
func ModuleReady(name, dest string) bool {
	if st, err := os.Stat(dest); err != nil || !st.IsDir() {
		return false
	}
	switch strings.ToLower(name) {
	case "apache":
		if _, err := FindBinary(dest, "httpd"); err == nil {
			return true
		}
		_, err := FindBinary(dest, "apache2")
		return err == nil
	case "mysql", "mariadb", "mysql-community":
		if _, err := FindBinary(dest, "mysqld"); err == nil {
			return true
		}
		_, err := FindBinary(dest, "mariadbd")
		return err == nil
	case "postgres", "postgresql":
		if _, err := FindBinary(dest, "postgres"); err == nil {
			return true
		}
		_, err := FindBinary(dest, "initdb")
		return err == nil
	case "composer":
		_, err := os.Stat(filepath.Join(dest, "composer.phar"))
		return err == nil
	case "xdebug":
		if _, err := os.Stat(filepath.Join(dest, "php_xdebug.dll")); err == nil {
			return true
		}
		_, err := os.Stat(filepath.Join(dest, "php_xdebug.so"))
		return err == nil
	case "phpmyadmin":
		_, err := os.Stat(filepath.Join(dest, "index.php"))
		return err == nil
	case "php":
		if _, err := FindBinary(dest, "php"); err == nil {
			return true
		}
		// php-builder ships versioned names (php8.1) until normalizePHPLayout runs.
		return findNamedFile(dest, rePHPCLI) != ""
	case "mailpit":
		_, err := FindBinary(dest, "mailpit")
		return err == nil
	case "mkcert":
		_, err := FindBinary(dest, "mkcert")
		return err == nil
	default:
		entries, err := os.ReadDir(dest)
		return err == nil && len(entries) > 0
	}
}

// DirFor returns install dir for a module on a target platform.
func (c *Cache) DirFor(name, version string, t Target) string {
	switch name {
	case "phpmyadmin", "composer":
		return filepath.Join(c.Root, name, version)
	default:
		osName, arch := t.OS, t.Arch
		if osName == "any" {
			osName = "any"
		}
		if arch == "any" {
			arch = "any"
		}
		return filepath.Join(c.Root, name, version, osName+"-"+arch)
	}
}

// Dir returns the install directory for the host platform.
func (c *Cache) Dir(name, version string) string {
	return c.DirFor(name, version, HostTarget())
}

func moduleKeyFor(name, version string, t Target) string {
	if version == "" {
		return fmt.Sprintf("%s@%s-%s", name, t.OS, t.Arch)
	}
	return fmt.Sprintf("%s-%s@%s-%s", name, version, t.OS, t.Arch)
}

// Ensure downloads for the host platform (re-downloads if cache incomplete).
func (c *Cache) Ensure(name, version string) (string, error) {
	return c.EnsureTarget(name, version, HostTarget(), false)
}

// EnsureTarget downloads/extracts a module for a platform.
func (c *Cache) EnsureTarget(name, version string, t Target, force bool) (string, error) {
	resolveOS, resolveArch := t.OS, t.Arch
	if resolveOS == "any" {
		resolveOS = "any"
	}
	if resolveArch == "any" {
		resolveArch = "any"
	}

	modVer, art, err := c.Catalog.ResolveFor(name, version, resolveOS, resolveArch)
	if err != nil && (t.OS != "any" || t.Arch != "any") {
		// Fall back to any/any (composer, phpmyadmin)
		modVer, art, err = c.Catalog.ResolveFor(name, version, "any", "any")
		if err == nil {
			t = Target{OS: "any", Arch: "any"}
		}
	}
	if err != nil {
		return "", err
	}

	dest := c.DirFor(name, modVer, t)
	key := moduleKeyFor(name, modVer, t)
	man := c.loadManifest()

	if !force && ModuleReady(name, dest) {
		ensureModuleExecutables(name, dest)
		if entry, ok := man.Modules[key]; ok && entry.URL != art.URL {
			logx.Warn("%s %s (%s): newer catalog URL available", name, modVer, t)
		} else {
			logx.Info("%s %s (%s): up to date (cached)", name, modVer, t)
		}
		return dest, nil
	}

	if !force {
		if st, err := os.Stat(dest); err == nil && st.IsDir() && !ModuleReady(name, dest) {
			logx.Warn("%s %s (%s): cache incomplete — re-downloading", name, modVer, t)
		}
	}
	_ = os.RemoveAll(dest)

	logx.Info("downloading %s %s (%s) …", name, modVer, t)
	if art.Note != "" {
		logx.Warn("%s: %s", name, art.Note)
	}

	tmpDir, err := os.MkdirTemp(c.Root, "dl-*")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tmpDir)

	archivePath := filepath.Join(tmpDir, art.Filename)
	sum, err := c.download(art.URL, archivePath)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	if err := validateArchiveMagic(archivePath, art.Kind); err != nil {
		_ = os.Remove(archivePath)
		return "", fmt.Errorf("download %s: %w", name, err)
	}
	if art.Checksum != "" && !strings.EqualFold(art.Checksum, sum) {
		return "", fmt.Errorf("checksum mismatch for %s: got %s want %s", name, sum, art.Checksum)
	}

	if err := os.MkdirAll(dest, 0o755); err != nil {
		return "", err
	}

	switch strings.ToLower(art.Kind) {
	case "file":
		target := filepath.Join(dest, art.Filename)
		if err := copyFile(archivePath, target); err != nil {
			_ = os.RemoveAll(dest)
			return "", err
		}
		if runtime.GOOS != "windows" && !strings.HasSuffix(strings.ToLower(art.Filename), ".dll") {
			_ = os.Chmod(target, 0o755)
		}
	case "zip":
		if err := extractZip(archivePath, dest, art.StripComponents); err != nil {
			_ = os.RemoveAll(dest)
			return "", err
		}
	case "tar.gz", "tgz":
		if err := extractTarGz(archivePath, dest, art.StripComponents); err != nil {
			_ = os.RemoveAll(dest)
			return "", err
		}
	case "tar.xz", "txz":
		if err := extractTarXz(archivePath, dest, art.StripComponents); err != nil {
			_ = os.RemoveAll(dest)
			return "", err
		}
	default:
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("unknown artifact kind %q", art.Kind)
	}

	if !ModuleReady(name, dest) {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("%s extracted but required files not found under %s", name, dest)
	}
	ensureModuleExecutables(name, dest)

	// Re-check after PHP alias normalization (php8.1 → php).
	if !ModuleReady(name, dest) {
		_ = os.RemoveAll(dest)
		return "", fmt.Errorf("%s extracted but required files not found under %s", name, dest)
	}

	man.Modules[key] = ManifestEntry{
		Version:  modVer,
		URL:      art.URL,
		Checksum: sum,
		Updated:  timeNowUTC(),
	}
	if err := c.saveManifest(man); err != nil {
		return "", err
	}
	logx.Success("%s %s (%s) installed → %s", name, modVer, t, dest)
	return dest, nil
}

func validateArchiveMagic(path, kind string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	buf := make([]byte, 6)
	n, _ := f.Read(buf)
	if n < 2 {
		return fmt.Errorf("downloaded file too small")
	}
	switch strings.ToLower(kind) {
	case "zip":
		if buf[0] != 'P' || buf[1] != 'K' {
			return fmt.Errorf("expected zip archive, got non-zip payload (site may have returned an error page)")
		}
	case "tar.gz", "tgz":
		if !(buf[0] == 0x1f && buf[1] == 0x8b) {
			return fmt.Errorf("expected gzip archive, got unexpected payload")
		}
	case "tar.xz", "txz":
		if !(n >= 6 && buf[0] == 0xfd && buf[1] == '7' && buf[2] == 'z' && buf[3] == 'X' && buf[4] == 'Z' && buf[5] == 0x00) {
			return fmt.Errorf("expected xz archive, got unexpected payload")
		}
	case "file":
		// DLL/exe/phar — reject obvious HTML
		if buf[0] == '<' || (n >= 5 && string(buf[:5]) == "<!DOC") {
			return fmt.Errorf("expected binary file, got HTML")
		}
	}
	return nil
}

// Prefetch runs cache jobs; continues after individual failures.
func (c *Cache) Prefetch(jobs []CacheJob) (ok, failed int) {
	for _, j := range jobs {
		t := j.Target
		_, err := c.EnsureTarget(j.Name, j.Version, t, false)
		if err != nil {
			failed++
			logx.Error("%s %s (%s): %v", j.Name, j.Version, j.Target, err)
			continue
		}
		ok++
	}
	return ok, failed
}
