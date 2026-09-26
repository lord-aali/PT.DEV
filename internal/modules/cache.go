package modules

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/pt-dev/ptdev/internal/logx"
)

// Cache manages the modules/ directory next to the binary.
type Cache struct {
	Root    string
	Catalog *Catalog
	Client  *http.Client
}

// Manifest tracks installed modules.
type Manifest struct {
	Modules map[string]ManifestEntry `json:"modules"`
}

// ManifestEntry is one cached module record (paths are derived from the binary location).
type ManifestEntry struct {
	Version  string    `json:"version"`
	URL      string    `json:"url"`
	Checksum string    `json:"checksum"`
	Updated  time.Time `json:"updated"`
}

func timeNowUTC() time.Time { return time.Now().UTC() }

// NewCache locates/creates the cache beside the executable.
func NewCache() (*Cache, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, err
	}
	exe, err = filepath.EvalSymlinks(exe)
	if err != nil {
		exe, _ = os.Executable()
	}
	root := filepath.Join(filepath.Dir(exe), "modules")
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, err
	}
	cat, err := LoadCatalog()
	if err != nil {
		return nil, err
	}
	c := &Cache{
		Root:    root,
		Catalog: cat,
		Client:  logx.Client(),
	}
	c.rewriteManifest()
	return c, nil
}

// AbsPath returns the absolute cache path for a module relative to the binary.
func (c *Cache) AbsPath(name, version string, t Target) string {
	return c.DirFor(name, version, t)
}

func (c *Cache) manifestPath() string {
	return filepath.Join(c.Root, "manifest.json")
}

func (c *Cache) loadManifest() Manifest {
	m := Manifest{Modules: map[string]ManifestEntry{}}
	data, err := os.ReadFile(c.manifestPath())
	if err != nil {
		return m
	}
	_ = json.Unmarshal(data, &m)
	if m.Modules == nil {
		m.Modules = map[string]ManifestEntry{}
	}
	return m
}

// rewriteManifest drops legacy fields (e.g. absolute "path") by round-tripping through the typed struct.
func (c *Cache) rewriteManifest() {
	man := c.loadManifest()
	if len(man.Modules) == 0 {
		return
	}
	_ = c.saveManifest(man)
}

func (c *Cache) saveManifest(m Manifest) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.manifestPath(), data, 0o644)
}

// InvalidateAll deletes every cached module under the modules/ root (after caller confirms).
func (c *Cache) InvalidateAll() error {
	entries, err := os.ReadDir(c.Root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		p := filepath.Join(c.Root, e.Name())
		if err := os.RemoveAll(p); err != nil {
			return fmt.Errorf("remove %s: %w", p, err)
		}
	}
	if err := os.MkdirAll(c.Root, 0o755); err != nil {
		return err
	}
	return c.saveManifest(Manifest{Modules: map[string]ManifestEntry{}})
}

func (c *Cache) download(url, dest string) (sha256hex string, err error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; ptdev/0.1; +https://github.com/pt-dev/ptdev)")
	req.Header.Set("Accept", "*/*")
	resp, err := c.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("HTTP %s", resp.Status)
	}
	ct := strings.ToLower(resp.Header.Get("Content-Type"))
	if strings.Contains(ct, "text/html") {
		return "", fmt.Errorf("got HTML instead of a binary from %s (link may be dead or blocked)", url)
	}

	f, err := os.Create(dest)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	w := io.MultiWriter(f, h)
	var written int64
	var lastReport int64
	buf := make([]byte, 32*1024)
	for {
		n, rerr := resp.Body.Read(buf)
		if n > 0 {
			if _, werr := w.Write(buf[:n]); werr != nil {
				return "", werr
			}
			written += int64(n)
			if resp.ContentLength > 0 && written-lastReport >= 5*1024*1024 {
				lastReport = written
				pct := float64(written) / float64(resp.ContentLength) * 100
				logx.Info("  … %s / %s (%.0f%%)", logx.FormatBytes(written), logx.FormatBytes(resp.ContentLength), pct)
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			return "", rerr
		}
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
