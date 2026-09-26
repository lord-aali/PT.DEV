package hosts

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

// Path returns the system hosts file path.
func Path() string {
	if runtime.GOOS == "windows" {
		return `C:\Windows\System32\drivers\etc\hosts`
	}
	return "/etc/hosts"
}

// Ensure maps hostname to 127.0.0.1 if missing. May require elevation.
func Ensure(hostname string) error {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "" {
		return nil
	}
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read hosts: %w (try running as administrator)", err)
	}
	marker := "# ptdev:" + hostname
	content := string(data)
	if strings.Contains(content, hostname) && strings.Contains(content, marker) {
		return nil
	}
	if strings.Contains(content, "\t"+hostname) || strings.Contains(content, " "+hostname) {
		// Already mapped somehow
		return nil
	}
	line := fmt.Sprintf("\n127.0.0.1\t%s\t%s\n", hostname, marker)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("write hosts: %w (try running as administrator/sudo)", err)
	}
	defer f.Close()
	if _, err := f.WriteString(line); err != nil {
		return err
	}
	return nil
}

// Remove removes a ptdev-managed hosts entry.
func Remove(hostname string) error {
	hostname = strings.TrimSpace(strings.ToLower(hostname))
	if hostname == "" {
		return nil
	}
	path := Path()
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	marker := "# ptdev:" + hostname
	lines := strings.Split(string(data), "\n")
	var out []string
	for _, line := range lines {
		if strings.Contains(line, marker) {
			continue
		}
		out = append(out, line)
	}
	return os.WriteFile(path, []byte(strings.Join(out, "\n")), 0o644)
}
