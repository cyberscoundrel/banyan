package sdk

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

// Middleware wraps an http.Handler.
type Middleware func(http.Handler) http.Handler

// Mux is a simple builder for registering endpoints with http-style handlers and middleware.
type Mux struct {
	logger      func(format string, v ...interface{})
	middlewares []Middleware
	routes      []muxRoute
}

type muxRoute struct {
	kind    string
	method  string
	path    string
	handler http.Handler
}

// NewMux creates a new Mux.
func NewMux() *Mux {
	return &Mux{logger: func(format string, v ...interface{}) {
		_, _ = io.WriteString(os.Stderr, fmt.Sprintf(format+"\n", v...))
	}}
}

// WithLogger sets a custom logger function.
func (m *Mux) WithLogger(logf func(format string, v ...interface{})) *Mux {
	m.logger = logf
	return m
}

// Use appends a middleware to the chain.
func (m *Mux) Use(mw Middleware) *Mux {
	if mw != nil {
		m.middlewares = append(m.middlewares, mw)
	}
	return m
}

// Local registers a local (management) endpoint.
func (m *Mux) Local(method, path string, h http.Handler) *Mux {
	m.routes = append(m.routes, muxRoute{kind: "local", method: methodOrAny(method), path: path, handler: h})
	return m
}

// Remote registers a remote (p2p) endpoint.
func (m *Mux) Remote(method, path string, h http.Handler) *Mux {
	m.routes = append(m.routes, muxRoute{kind: "remote", method: methodOrAny(method), path: path, handler: h})
	return m
}

// GET/POST helpers
func (m *Mux) GET(path string, h http.Handler) *Mux   { return m.Local(http.MethodGet, path, h) }
func (m *Mux) POST(path string, h http.Handler) *Mux  { return m.Local(http.MethodPost, path, h) }
func (m *Mux) RGET(path string, h http.Handler) *Mux  { return m.Remote(http.MethodGet, path, h) }
func (m *Mux) RPOST(path string, h http.Handler) *Mux { return m.Remote(http.MethodPost, path, h) }

func methodOrAny(m string) string {
	if m == "" {
		return "*"
	}
	return m
}

// endpoints converts mux routes to SDK endpoints.
func (m *Mux) endpoints() []Endpoint {
	endpoints := make([]Endpoint, 0, len(m.routes))
	for _, r := range m.routes {
		// Build the handler chain once per route
		final := r.handler
		for i := len(m.middlewares) - 1; i >= 0; i-- {
			final = m.middlewares[i](final)
		}
		wrapped := func(req *HTTPRequest) (*HTTPResponse, error) {
			// Convert to http.Request and capture response
			u := &url.URL{Scheme: "http", Host: "addon.local", Path: req.Path, RawQuery: req.RawQuery}
			bodyBytes := decodeBase64(req.BodyBase64)
			hreq, _ := http.NewRequest(req.Method, u.String(), bytes.NewReader(bodyBytes))
			for k, vv := range req.Headers {
				for _, v := range vv {
					hreq.Header.Add(k, v)
				}
			}
			hreq = withPathParams(hreq, req.PathParams)
			w := newResponseBuffer()
			start := time.Now()
			final.ServeHTTP(w, hreq)
			if m.logger != nil {
				m.logger("%s %s -> %d %dB (%s)", req.Method, req.Path, w.Status(), w.Size(), time.Since(start))
			}
			return &HTTPResponse{Status: w.Status(), Headers: w.Header(), BodyBase64: encodeBase64(w.Bytes())}, nil
		}
		endpoints = append(endpoints, Endpoint{Kind: r.kind, Method: r.method, Path: r.path, Handle: wrapped})
	}
	return endpoints
}

// Logger is a middleware that logs request and response.
func Logger(logf func(format string, v ...interface{})) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			next.ServeHTTP(w, r)
			if logf != nil {
				status := 0
				size := 0
				if rb, ok := w.(interface {
					Status() int
					Size() int
				}); ok {
					status = rb.Status()
					size = rb.Size()
				}
				logf("%s %s -> %d %dB (%s)", r.Method, r.URL.Path, status, size, time.Since(start))
			}
		})
	}
}

// Helpers for common operations in handlers

func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

func WriteText(w http.ResponseWriter, status int, body string) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

func BindJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// responseBuffer captures http responses in-memory
type responseBuffer struct {
	headers http.Header
	status  int
	buf     bytes.Buffer
}

func newResponseBuffer() *responseBuffer {
	return &responseBuffer{headers: make(http.Header), status: http.StatusOK}
}

func (r *responseBuffer) Header() http.Header         { return r.headers }
func (r *responseBuffer) WriteHeader(code int)        { r.status = code }
func (r *responseBuffer) Write(b []byte) (int, error) { return r.buf.Write(b) }
func (r *responseBuffer) Status() int                 { return r.status }
func (r *responseBuffer) Bytes() []byte               { return r.buf.Bytes() }
func (r *responseBuffer) Size() int                   { return r.buf.Len() }

// internal helpers
type ctxKey int

const pathParamsKey ctxKey = 1

func withPathParams(r *http.Request, pp map[string]string) *http.Request {
	if pp == nil {
		return r
	}
	ctx := context.WithValue(r.Context(), pathParamsKey, pp)
	return r.WithContext(ctx)
}

// Param gets a path parameter by name, returns empty string if missing.
func Param(r *http.Request, name string) string {
	pp, _ := r.Context().Value(pathParamsKey).(map[string]string)
	if pp == nil {
		return ""
	}
	return pp[name]
}

func encodeBase64(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return base64.StdEncoding.EncodeToString(b)
}
func decodeBase64(s string) []byte {
	if s == "" {
		return nil
	}
	bb, _ := base64.StdEncoding.DecodeString(s)
	return bb
}
