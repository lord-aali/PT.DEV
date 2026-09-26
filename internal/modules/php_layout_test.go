package modules

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalizePHPLayout(t *testing.T) {
	dir := t.TempDir()
	bin := filepath.Join(dir, "usr", "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(bin, "php8.1")
	if err := os.WriteFile(src, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	cgi := filepath.Join(bin, "php-cgi8.1")
	if err := os.WriteFile(cgi, []byte("cgi"), 0o755); err != nil {
		t.Fatal(err)
	}

	if ModuleReady("php", dir) != true {
		t.Fatal("expected ready via versioned php8.1")
	}
	normalizePHPLayout(dir)
	if _, err := os.Stat(filepath.Join(bin, "php")); err != nil {
		t.Fatalf("php alias missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(bin, "php-cgi")); err != nil {
		t.Fatalf("php-cgi alias missing: %v", err)
	}
	p, err := FindBinary(dir, "php")
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(p) != "php" && filepath.Base(p) != "php8.1" {
		t.Fatalf("unexpected php path %s", p)
	}
}
