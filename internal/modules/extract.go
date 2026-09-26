package modules

import (
	"archive/tar"
	"archive/zip"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/ulikunitz/xz"
)

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

func extractZip(src, dest string, strip int) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()
	for _, f := range r.File {
		name := stripPath(f.Name, strip)
		if name == "" || name == "." {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("zip slip: %s", f.Name)
		}
		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			_ = rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		_ = out.Close()
		_ = rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

func extractTarGz(src, dest string, strip int) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gz.Close()
	return extractTar(gz, dest, strip)
}

func extractTarXz(src, dest string, strip int) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	xzr, err := xz.NewReader(f)
	if err != nil {
		return err
	}
	return extractTar(xzr, dest, strip)
}

func extractTar(r io.Reader, dest string, strip int) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := stripPath(hdr.Name, strip)
		if name == "" || name == "." {
			continue
		}
		target := filepath.Join(dest, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target), filepath.Clean(dest)+string(os.PathSeparator)) &&
			filepath.Clean(target) != filepath.Clean(dest) {
			return fmt.Errorf("tar slip: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg, tar.TypeRegA:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, os.FileMode(hdr.Mode))
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			_ = out.Close()
			if err != nil {
				return err
			}
		case tar.TypeSymlink, tar.TypeLink:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			_ = os.Remove(target)
			linkTarget := hdr.Linkname
			if hdr.Typeflag == tar.TypeLink {
				linkTarget = filepath.Join(dest, stripPath(hdr.Linkname, strip))
			}
			if err := os.Symlink(linkTarget, target); err != nil {
				// Windows without symlink privilege: materialize a copy when possible.
				src := linkTarget
				if !filepath.IsAbs(src) {
					src = filepath.Join(filepath.Dir(target), linkTarget)
				}
				if copyErr := copyFile(src, target); copyErr != nil {
					// Non-fatal: versioned binaries (php8.1) are enough; aliases added later.
					continue
				}
			}
		}
	}
	return nil
}

func stripPath(name string, n int) string {
	name = strings.TrimPrefix(name, "./")
	parts := strings.Split(name, "/")
	if n <= 0 {
		return name
	}
	if len(parts) <= n {
		return ""
	}
	return strings.Join(parts[n:], "/")
}

// FindBinary walks dir for a named executable (with .exe on Windows).
// For "php" / "php-cgi" / "php-fpm", also accepts versioned php-builder names (php8.1).
func FindBinary(dir, name string) (string, error) {
	candidates := []string{name}
	if runtime.GOOS == "windows" {
		candidates = []string{name + ".exe", name}
	}
	var found string
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || found != "" {
			return nil
		}
		base := info.Name()
		baseLower := strings.ToLower(base)
		for _, c := range candidates {
			if baseLower == strings.ToLower(c) {
				found = path
				return io.EOF
			}
		}
		return nil
	})
	if found == "" {
		switch name {
		case "php":
			found = findNamedFile(dir, rePHPCLI)
		case "php-cgi":
			found = findNamedFile(dir, rePHPCGI)
		case "php-fpm":
			found = findNamedFile(dir, rePHPFPM)
		}
	}
	if found == "" {
		return "", fmt.Errorf("%s not found under %s", name, dir)
	}
	_ = ensureUnixExec(found)
	return found, nil
}
