package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestModuleReadyEmptyDir(t *testing.T) {
	dir := t.TempDir()
	if ModuleReady("apache", dir) {
		t.Fatal("empty dir should not be ready")
	}
}

func TestEnsureApacheWindows(t *testing.T) {
	if os.Getenv("PTDEV_NET_TEST") == "" {
		t.Skip("set PTDEV_NET_TEST=1 to download apache")
	}
	c, err := NewCache()
	if err != nil {
		t.Fatal(err)
	}
	// use temp cache root
	c.Root = filepath.Join(t.TempDir(), "modules")
	_ = os.MkdirAll(c.Root, 0o755)

	dest, err := c.Ensure("apache", "")
	if err != nil {
		t.Fatal(err)
	}
	bin, err := FindBinary(dest, "httpd")
	if err != nil {
		t.Fatal(err)
	}
	t.Log("httpd=", bin)
}
