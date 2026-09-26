package config

import (
	"os"
	"path/filepath"
	"strings"
)

const gitignoreSnippet = `
# PT.DEV
.pt.dev/mysql-data/
.pt.dev/postgres-data/
.pt.dev/runtime/
.pt.dev/logs/
.pt.dev/tmp/
.pt.dev/launch.lock
`

// EnsureGitignore appends PT.DEV ignore rules if missing.
func EnsureGitignore(projectRoot string) error {
	path := filepath.Join(projectRoot, ".gitignore")
	existing := ""
	if data, err := os.ReadFile(path); err == nil {
		existing = string(data)
	}
	if strings.Contains(existing, ".pt.dev/mysql-data/") && strings.Contains(existing, ".pt.dev/postgres-data/") {
		return nil
	}
	// Older projects may only ignore mysql-data — append postgres-data if missing.
	if strings.Contains(existing, ".pt.dev/mysql-data/") && !strings.Contains(existing, ".pt.dev/postgres-data/") {
		f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
		if err != nil {
			return err
		}
		defer f.Close()
		if !strings.HasSuffix(existing, "\n") {
			if _, err := f.WriteString("\n"); err != nil {
				return err
			}
		}
		_, err = f.WriteString(".pt.dev/postgres-data/\n")
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if existing != "" && !strings.HasSuffix(existing, "\n") {
		if _, err := f.WriteString("\n"); err != nil {
			return err
		}
	}
	_, err = f.WriteString(gitignoreSnippet)
	return err
}
