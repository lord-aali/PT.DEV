package envfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pt-dev/ptdev/internal/config"
)

// WriteDBKeys creates or updates DB_* keys in project .env.
func WriteDBKeys(cfg *config.Config) error {
	if !cfg.MySQL.Enabled {
		return nil
	}
	path := filepath.Join(cfg.Location, ".env")

	vals := map[string]string{
		"DB_CONNECTION": cfg.MySQL.ConnectionDriver(),
		"DB_HOST":       "127.0.0.1",
		"DB_PORT":       fmt.Sprintf("%d", cfg.MySQL.Port),
		"DB_DATABASE":   cfg.MySQL.Database,
		"DB_USERNAME":   cfg.MySQL.User,
		"DB_PASSWORD":   cfg.MySQL.Password,
	}

	var b strings.Builder
	seen := map[string]bool{}

	if data, err := os.ReadFile(path); err == nil {
		lines := strings.Split(string(data), "\n")
		for i, line := range lines {
			// Preserve line endings roughly
			trim := strings.TrimSpace(line)
			if trim == "" || strings.HasPrefix(trim, "#") {
				b.WriteString(line)
				if i < len(lines)-1 || strings.HasSuffix(string(data), "\n") {
					b.WriteByte('\n')
				}
				continue
			}
			k, _, ok := strings.Cut(line, "=")
			if !ok {
				b.WriteString(line)
				b.WriteByte('\n')
				continue
			}
			k = strings.TrimSpace(k)
			if v, hit := vals[k]; hit {
				b.WriteString(k)
				b.WriteByte('=')
				b.WriteString(quoteIfNeeded(v))
				b.WriteByte('\n')
				seen[k] = true
				continue
			}
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}

	// Stable order for new keys
	order := []string{"DB_CONNECTION", "DB_HOST", "DB_PORT", "DB_DATABASE", "DB_USERNAME", "DB_PASSWORD"}
	for _, k := range order {
		if seen[k] {
			continue
		}
		b.WriteString(k)
		b.WriteByte('=')
		b.WriteString(quoteIfNeeded(vals[k]))
		b.WriteByte('\n')
		seen[k] = true
	}

	return os.WriteFile(path, []byte(b.String()), 0o644)
}

func quoteIfNeeded(v string) string {
	if v == "" {
		return `""`
	}
	if strings.ContainsAny(v, " \t#\"'") {
		return `"` + strings.ReplaceAll(v, `"`, `\"`) + `"`
	}
	return v
}
