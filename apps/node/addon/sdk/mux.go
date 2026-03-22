// Package sdk provides an HTTP multiplexer and routing utilities for building Banyan addons.
// The Mux type allows addons to register HTTP handlers using standard net/http signatures,
// with support for middleware chaining and distinction between local (management) and
// remote (p2p) endpoint kinds.
//
// The Mux converts registered routes into SDK Endpoints that can be run via Client.RunMux.
// This allows addon authors to use familiar http.Handler patterns while the SDK handles
// the JSON-RPC communication with the host manager.
//
// Example usage:
//
//	mux := sdk.NewMux()
//	mux.GET("/my-addon/hello", http.HandlerFunc(handleHello))
//	mux.POST("/my-addon/data", http.HandlerFunc(handleData))
//	mux.Use(sdk.Logger(log.Printf))
//	client.RunMux(mux)
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

// Middleware is a function that wraps an http.Handler to add cross-cutting concerns
// such as logging, authentication, or request modification.
type Middleware func(http.Handler) http.Handler

// Mux is a builder for registering HTTP endpoints with standard http.Handler signatures.
// It supports middleware chaining and distinguishes between local (management) and
// remote (p2p) endpoint kinds. Routes are converted to SDK Endpoints for the Client.
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

// NewMux creates a new Mux with a default logger that writes to stderr.
func NewMux() *Mux {
	return &Mux{logger: func(format string, v ...interface{}) {
		_, _ = io.WriteString(os.Stderr, fmt.Sprintf(format+"\n", v...))
	}}
}

// WithLogger sets a custom logging function for request logging.
func (m *Mux) WithLogger(logf func(format string, v ...interface{})) *Mux {
	m.logger = logf
	return m
}

// Use appends middleware to the chain. Middleware is applied in registration order.
func (m *Mux) Use(mw Middleware) *Mux {
	if mw != nil {
		m.middlewares = append(m.middlewares, mw)
	}
	return m
}

// Local registers a handler on the local (management) endpoint with the given method and path.
func (m *Mux) Local(method, path string, h http.Handler) *Mux {
	m.routes = append(m.routes, muxRoute{kind: "local", method: methodOrAny(method), path: path, handler: h})
	return m
}

// Remote registers a handler on the remote (p2p) endpoint with the given method and path.
func (m *Mux) Remote(method, path string, h http.Handler) *Mux {
	m.routes = append(m.routes, muxRoute{kind: "remote", method: methodOrAny(method), path: path, handler: h})
	return m
}

// GET registers a local GET endpoint. It is a shorthand for Local(http.MethodGet, path, h).
func (m *Mux) GET(path string, h http.Handler) *Mux { return m.Local(http.MethodGet, path, h) }

// POST registers a local POST endpoint. It is a shorthand for Local(http.MethodPost, path, h).
func (m *Mux) POST(path string, h http.Handler) *Mux { return m.Local(http.MethodPost, path, h) }

// RGET registers a remote GET endpoint. It is a shorthand for Remote(http.MethodGet, path, h).
func (m *Mux) RGET(path string, h http.Handler) *Mux { return m.Remote(http.MethodGet, path, h) }

// RPOST registers a remote POST endpoint. It is a shorthand for Remote(http.MethodPost, path, h).
func (m *Mux) RPOST(path string, h http.Handler) *Mux { return m.Remote(http.MethodPost, path, h) }

func methodOrAny(m string) string {
	if m == "" {
		return "*"
	}
	return m
}

// endpoints converts Mux routes to SDK Endpoints for use with Client.Run.
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

// Logger returns middleware that logs each request with method, path, status, size, and duration.
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

// WriteJSON writes a JSON-encoded response with the given status code and content type header.
// If status is 0, http.StatusOK (200) is used.
func WriteJSON(w http.ResponseWriter, status int, v any) {
	b, _ := json.Marshal(v)
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write(b)
}

// WriteText writes a text/plain response with the given status code.
// If status is 0, http.StatusOK (200) is used.
func WriteText(w http.ResponseWriter, status int, body string) {
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "text/plain")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, body)
}

// BindJSON decodes a JSON request body into the provided value and closes the body.
func BindJSON(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// responseBuffer captures http.ResponseWriter output in memory for conversion to HTTPResponse.
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

// Param retrieves a path parameter value by name from the request context.
// Returns an empty string if the parameter is not found.
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
