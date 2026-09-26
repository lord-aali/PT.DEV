package logx

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
)

const maxBodyPeek = 2048

// Transport wraps an http.RoundTripper with verbose/debug logging.
type Transport struct {
	Base http.RoundTripper
}

func (t *Transport) base() http.RoundTripper {
	if t.Base != nil {
		return t.Base
	}
	return http.DefaultTransport
}

// RoundTrip implements http.RoundTripper.
func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	FetchMethodURL(req.Method, req.URL.String())

	if IsDebug() {
		dumpHeaders("request headers", req.Header)
		if req.Body != nil && req.GetBody != nil {
			b, err := req.GetBody()
			if err == nil {
				dumpBody("request body", b, -1)
			}
		} else if req.Body != nil {
			var buf bytes.Buffer
			tee := io.TeeReader(req.Body, &buf)
			data, _ := io.ReadAll(io.LimitReader(tee, maxBodyPeek+1))
			rest, _ := io.ReadAll(tee)
			req.Body = io.NopCloser(io.MultiReader(bytes.NewReader(data), bytes.NewReader(rest)))
			dumpBodyBytes("request body", data, int64(len(data)+len(rest)))
		}
	}

	resp, err := t.base().RoundTrip(req)
	if err != nil {
		Error("request failed: %v", err)
		return nil, err
	}

	if IsDebug() {
		DebugStatus("%s %s", resp.Status, req.URL.String())
		dumpHeaders("response headers", resp.Header)
		cl := resp.ContentLength
		ct := resp.Header.Get("Content-Type")
		if isBinaryContent(ct) || cl > maxBodyPeek {
			Debug("response body: binary/large (Content-Type=%s Content-Length=%d) — peeking first %d bytes",
				ct, cl, maxBodyPeek)
			peekAndRestore(&resp.Body, maxBodyPeek)
		} else {
			var buf bytes.Buffer
			_, _ = io.Copy(&buf, io.LimitReader(resp.Body, maxBodyPeek+1))
			rest, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()
			all := append(buf.Bytes(), rest...)
			dumpBodyBytes("response body", all, int64(len(all)))
			resp.Body = io.NopCloser(bytes.NewReader(all))
		}
	}

	return resp, nil
}

func dumpHeaders(label string, h http.Header) {
	Debug("%s:", label)
	for k, vals := range h {
		Debug("  %s: %s", k, strings.Join(vals, ", "))
	}
}

func dumpBody(label string, r io.Reader, total int64) {
	data, _ := io.ReadAll(io.LimitReader(r, maxBodyPeek+1))
	dumpBodyBytes(label, data, total)
}

func dumpBodyBytes(label string, data []byte, total int64) {
	truncated := len(data) > maxBodyPeek
	if truncated {
		data = data[:maxBodyPeek]
	}
	text := string(data)
	if !isPrintable(text) {
		Debug("%s: <%d bytes binary>", label, total)
		return
	}
	if truncated || (total > 0 && total > int64(len(data))) {
		Debug("%s (truncated):\n%s\n… (%d bytes total)", label, text, total)
		return
	}
	Debug("%s:\n%s", label, text)
}

func peekAndRestore(body *io.ReadCloser, n int) {
	if body == nil || *body == nil {
		return
	}
	buf := make([]byte, n)
	read, _ := io.ReadFull(*body, buf)
	buf = buf[:read]
	if isPrintable(string(buf)) {
		Debug("body peek:\n%s", string(buf))
	} else {
		Debug("body peek: <%d bytes binary>", read)
	}
	*body = io.NopCloser(io.MultiReader(bytes.NewReader(buf), *body))
}

func isBinaryContent(ct string) bool {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "octet-stream"),
		strings.Contains(ct, "zip"),
		strings.Contains(ct, "gzip"),
		strings.Contains(ct, "x-tar"),
		strings.Contains(ct, "x-msdownload"),
		strings.HasPrefix(ct, "application/") && !strings.Contains(ct, "json") && !strings.Contains(ct, "xml") && !strings.Contains(ct, "text"):
		return true
	default:
		return false
	}
}

func isPrintable(s string) bool {
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' {
			continue
		}
		if r < 32 || r == 127 {
			return false
		}
	}
	return true
}

// Client returns an *http.Client that logs according to the active level.
func Client() *http.Client {
	return &http.Client{Transport: &Transport{}}
}

// FormatBytes is a small helper for progress messages.
func FormatBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for v := n / unit; v >= unit; v /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}
