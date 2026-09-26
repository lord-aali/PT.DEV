package modules

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestEnsureUnixExec(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "tool")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho ok\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ensureUnixExec(p); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(p)
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&0o100 == 0 {
		t.Fatalf("expected executable bit, got %v", st.Mode())
	}
}

func TestEnsureModuleExecutablesMkcert(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix only")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "mkcert")
	if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	ensureModuleExecutables("mkcert", dir)
	st, _ := os.Stat(p)
	if st.Mode()&0o100 == 0 {
		t.Fatalf("mkcert should be executable, mode=%v", st.Mode())
	}
}
