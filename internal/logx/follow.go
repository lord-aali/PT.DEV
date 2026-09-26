package logx

import (
	"bufio"
	"context"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// commonLogRE matches the request + status portion of Apache Common Log Format.
// Example: 127.0.0.1 - - [25/Sep/2026:03:00:00 +0000] "GET /foo HTTP/1.1" 200 1234
var commonLogRE = regexp.MustCompile(`"([A-Z]+) ([^"]*) HTTP/[^"]+"\s+(\d{3})`)

// FollowAccessLogs tails Apache common access log (verbose+) and error logs (debug).
func FollowAccessLogs(ctx context.Context, logsDir string) {
	if !IsVerbose() {
		return
	}
	access := filepath.Join(logsDir, "apache-access.log")
	_ = touchFile(access)
	Verbose("streaming [http] from Apache common access log")
	go followAccessCommon(ctx, access)

	if !IsDebug() {
		return
	}
	type job struct {
		path  string
		label string
	}
	jobs := []job{
		{filepath.Join(logsDir, "apache.log"), "apache"},
		{filepath.Join(logsDir, "apache-stdout.log"), "httpd"},
		{filepath.Join(logsDir, "php.log"), "php"},
	}
	Verbose("streaming error logs from %s", logsDir)
	for _, j := range jobs {
		j := j
		go followFile(ctx, j.path, j.label, true)
	}
}

func touchFile(path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	return f.Close()
}

func followAccessCommon(ctx context.Context, path string) {
	followFileFn(ctx, path, func(line string) {
		method, uri, status, ok := parseCommonLog(line)
		if ok {
			HTTP(method, uri, status)
			return
		}
		// Unparseable line — still surface it.
		if IsDebug() {
			Debug("[http] %s", line)
		} else {
			Verbose("[http] %s", line)
		}
	})
}

func parseCommonLog(line string) (method, uri, status string, ok bool) {
	m := commonLogRE.FindStringSubmatch(line)
	if m == nil {
		return "", "", "", false
	}
	uri = m[2]
	if i := strings.IndexByte(uri, ' '); i >= 0 {
		uri = uri[:i]
	}
	return m[1], uri, m[3], true
}

func followFile(ctx context.Context, path, label string, debugOnly bool) {
	followFileFn(ctx, path, func(line string) {
		if debugOnly {
			Debug("[%s] %s", label, line)
		} else {
			Verbose("[%s] %s", label, line)
		}
	})
}

func followFileFn(ctx context.Context, path string, onLine func(string)) {
	var f *os.File
	for f == nil {
		select {
		case <-ctx.Done():
			return
		default:
		}
		var err error
		f, err = os.Open(path)
		if err != nil {
			time.Sleep(250 * time.Millisecond)
			continue
		}
	}
	defer f.Close()

	_, _ = f.Seek(0, io.SeekEnd)
	r := bufio.NewReader(f)
	var offset int64
	if st, err := f.Stat(); err == nil {
		offset = st.Size()
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		if st, err := os.Stat(path); err == nil {
			if st.Size() < offset {
				_, _ = f.Seek(0, io.SeekStart)
				r = bufio.NewReader(f)
				offset = 0
			}
		}

		line, err := r.ReadString('\n')
		if n := len(line); n > 0 {
			offset += int64(n)
			line = strings.TrimRight(line, "\r\n")
			if line != "" {
				onLine(line)
			}
		}
		if err == io.EOF {
			time.Sleep(200 * time.Millisecond)
			continue
		}
		if err != nil {
			_ = f.Close()
			f = nil
			for f == nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
				var oerr error
				f, oerr = os.Open(path)
				if oerr != nil {
					time.Sleep(250 * time.Millisecond)
					continue
				}
				r = bufio.NewReader(f)
				if st, err := f.Stat(); err == nil {
					offset = st.Size()
					_, _ = f.Seek(0, io.SeekEnd)
				}
			}
		}
	}
}
