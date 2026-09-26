package httpproxy

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/pt-dev/ptdev/internal/logx"
)

const maxBody = 2048

// Server is a reverse proxy that logs method/URL (and bodies at debug).
type Server struct {
	srv *http.Server
}

// Start listens on publicAddr and forwards to backendURL (e.g. http://127.0.0.1:18080).
func Start(publicAddr, backendURL string) (*Server, error) {
	target, err := url.Parse(backendURL)
	if err != nil {
		return nil, err
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
		logx.Error("proxy: %v", e)
		http.Error(w, "bad gateway", http.StatusBadGateway)
	}
	proxy.FlushInterval = 100 * time.Millisecond

	h := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		full := r.URL.RequestURI()
		// Method/URL come from Apache Common access log ([http]). Proxy only adds bodies.

		var reqBody []byte
		if logx.IsDebug() && r.Body != nil {
			reqBody, _ = io.ReadAll(io.LimitReader(r.Body, maxBody+1))
			rest, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			all := append(reqBody, rest...)
			r.Body = io.NopCloser(bytes.NewReader(all))
			r.ContentLength = int64(len(all))
			r.GetBody = func() (io.ReadCloser, error) {
				return io.NopCloser(bytes.NewReader(all)), nil
			}
			dumpHeaders("request headers", r.Header)
			dumpBody("request body", reqBody, int64(len(all)))
		}

		rec := &recorder{ResponseWriter: w, status: 200}
		proxy.ServeHTTP(rec, r)

		if logx.IsDebug() {
			logx.Debug("[http] body-proxy ← %d %s %s", rec.status, r.Method, full)
			dumpHeaders("response headers", rec.Header())
			dumpBody("response body", rec.body.Bytes(), int64(rec.body.Len()))
		}
	})

	ln, err := net.Listen("tcp", publicAddr)
	if err != nil {
		return nil, err
	}
	s := &Server{srv: &http.Server{Handler: h}}
	go func() {
		err := s.srv.Serve(ln)
		if err != nil && err != http.ErrServerClosed {
			logx.Error("http log proxy: %v", err)
		}
	}()
	logx.Verbose("HTTP log proxy listening on %s → %s", publicAddr, backendURL)
	return s, nil
}

// Shutdown stops the proxy.
func (s *Server) Shutdown(ctx context.Context) error {
	if s == nil || s.srv == nil {
		return nil
	}
	return s.srv.Shutdown(ctx)
}

type recorder struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(p []byte) (int, error) {
	if r.body.Len() < maxBody {
		need := maxBody - r.body.Len()
		if need > len(p) {
			need = len(p)
		}
		_, _ = r.body.Write(p[:need])
	}
	return r.ResponseWriter.Write(p)
}

func dumpHeaders(label string, h http.Header) {
	logx.Debug("%s:", label)
	for k, vals := range h {
		logx.Debug("  %s: %s", k, strings.Join(vals, ", "))
	}
}

func dumpBody(label string, data []byte, total int64) {
	truncated := len(data) > maxBody
	if truncated {
		data = data[:maxBody]
	}
	if len(data) == 0 {
		logx.Debug("%s: <empty>", label)
		return
	}
	if !isPrintable(string(data)) {
		logx.Debug("%s: <%d bytes binary>", label, total)
		return
	}
	if truncated || total > int64(len(data)) {
		logx.Debug("%s (truncated):\n%s\n… (%d bytes total)", label, string(data), total)
		return
	}
	logx.Debug("%s:\n%s", label, string(data))
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

// Addr formats host:port for Listen.
func Addr(host string, port int) string {
	if host == "" {
		host = "0.0.0.0"
	}
	return fmt.Sprintf("%s:%d", host, port)
}
