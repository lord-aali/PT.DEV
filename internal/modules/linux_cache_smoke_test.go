package modules

import (
	"os"
	"testing"
)

func TestEnsureLinuxPHPApache(t *testing.T) {
	if os.Getenv("PTDEV_NET_TEST") == "" {
		t.Skip("set PTDEV_NET_TEST=1")
	}
	c, err := NewCache()
	if err != nil {
		t.Fatal(err)
	}
	t.Log("cache root", c.Root)
	dest, err := c.EnsureTarget("php", "8.1", Target{OS: "linux", Arch: "amd64"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := FindBinary(dest, "php"); err != nil {
		t.Fatal(err)
	} else {
		t.Log("php=", p)
	}
	dest, err = c.EnsureTarget("apache", "", Target{OS: "linux", Arch: "amd64"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if p, err := FindBinary(dest, "httpd"); err != nil {
		t.Fatal(err)
	} else {
		t.Log("httpd=", p)
	}
}
