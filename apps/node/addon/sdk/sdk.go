package sdk

import (
	"bufio"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// HTTPRequest represents a forwarded HTTP request from the host.
type HTTPRequest struct {
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	RawQuery   string              `json:"raw_query,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	BodyBase64 string              `json:"body_base64,omitempty"`
	PathParams map[string]string   `json:"path_params,omitempty"`
}

// HTTPResponse is returned by addon handlers to the host.
type HTTPResponse struct {
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers,omitempty"`
	BodyBase64 string              `json:"body_base64,omitempty"`
}

// Handler handles a forwarded HTTP request.
type Handler func(r *HTTPRequest) (*HTTPResponse, error)

// Endpoint describes a registration for an endpoint.
type Endpoint struct {
	Kind   string // "local" or "remote"
	Method string // GET/POST/etc, or "*"
	Path   string // supports trailing {param}
	Handle Handler
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Client wires stdin/stdout JSON-RPC and dispatches addon handlers.
type Client struct {
	enc     *json.Encoder
	dec     *json.Decoder
	mu      sync.Mutex
	started bool

	// path -> method -> handler
	handlersMu sync.RWMutex
	handlers   map[string]map[string]Handler

	// alias resolvers
	aliasMu   sync.RWMutex
	resolvers []func(string) (string, error)
	// extended resolvers can return either a path or raw JSON string
	resolversEx []func(string) (string, string, error)
}

// New creates a new addon SDK client bound to stdin/stdout.
func New() *Client {
	return &Client{
		enc:      json.NewEncoder(os.Stdout),
		dec:      json.NewDecoder(bufio.NewReader(os.Stdin)),
		handlers: make(map[string]map[string]Handler),
	}
}

// Run enters the main loop. It blocks until EOF or fatal decode error.
func (c *Client) Run(endpoints ...Endpoint) error {
	c.mu.Lock()
	if c.started {
		c.mu.Unlock()
		return errors.New("sdk client already started")
	}
	c.started = true
	c.mu.Unlock()

	// Pre-load handlers and registrations to send after host_ready.
	regs := make([]Endpoint, 0, len(endpoints)+1)
	for _, ep := range endpoints {
		if ep.Kind == "" {
			ep.Kind = "local"
		}
		if ep.Method == "" {
			ep.Method = "*"
		}
		if ep.Path == "" || ep.Handle == nil {
			continue
		}
		c.addHandler(ep.Path, ep.Method, ep.Handle)
		regs = append(regs, ep)
	}
	// Reserved alias resolver endpoint if any resolvers registered
	if c.hasAliasResolvers() {
		const aliasPath = "/addons/aliases/resolve/{alias}"
		c.addHandler(aliasPath, "GET", c.aliasResolveHandler())
		regs = append(regs, Endpoint{Kind: "local", Method: "GET", Path: aliasPath})
	}

	for {
		var m map[string]json.RawMessage
		if err := c.dec.Decode(&m); err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("decode: %w", err)
		}
		if methodRaw, ok := m["method"]; ok {
			method := stringTrimQuotes(methodRaw)
			switch method {
			case "host_ready":
				// Register endpoints
				for i := range regs {
					_ = c.send(map[string]any{
						"jsonrpc": "2.0",
						"id":      fmt.Sprintf("reg-%d", i+1),
						"method":  "addon_register_endpoint",
						"params": map[string]string{
							"kind":   regs[i].Kind,
							"method": regs[i].Method,
							"path":   regs[i].Path,
						},
					})
				}
				// If addon has disclosure info set before start, send it now
				if name := os.Getenv("BANYAN_ADDON_NAME"); name != "" {
					_ = c.send(map[string]any{
						"jsonrpc": "2.0",
						"id":      "disclose-1",
						"method":  "addon_disclose",
						"params": map[string]any{
							"name":    name,
							"version": os.Getenv("BANYAN_ADDON_VERSION"),
						},
					})
				}
			case "addon_handle_http":
				var req HTTPRequest
				_ = json.Unmarshal(m["params"], &req)
				handler := c.lookupHandler(req.Path, req.Method)
				if handler == nil {
					c.writeError(m["id"], -32601, "no handler")
					continue
				}
				resp, err := handler(&req)
				if err != nil {
					c.writeError(m["id"], -32000, err.Error())
					continue
				}
				b, _ := json.Marshal(resp)
				_ = c.send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": json.RawMessage(b)})
			case "alias_resolve":
				// Host requests alias resolution via RPC
				var in struct {
					Alias string `json:"alias"`
				}
				_ = json.Unmarshal(m["params"], &in)
				alias := strings.TrimSpace(in.Alias)
				if alias == "" {
					c.writeError(m["id"], -32602, "missing alias")
					continue
				}
				path, raw, err := c.runAliasResolvers(alias)
				if err != nil || (path == "" && raw == "") {
					// Nothing found
					c.writeError(m["id"], -32004, "alias not resolved")
					continue
				}
				out := map[string]any{"alias": alias}
				if path != "" {
					out["path"] = path
				}
				if raw != "" {
					out["json"] = raw
				}
				_ = c.send(map[string]any{"jsonrpc": "2.0", "id": m["id"], "result": out})
			default:
				// Unknown method from host: reply method not found if request has id
				if _, hasID := m["id"]; hasID {
					c.writeError(m["id"], -32601, "method not found")
				}
			}
			continue
		}
		// Responses are ignored by the SDK for now
	}
}

func (c *Client) addHandler(path, method string, h Handler) {
	c.handlersMu.Lock()
	defer c.handlersMu.Unlock()
	mm := c.handlers[path]
	if mm == nil {
		mm = make(map[string]Handler)
		c.handlers[path] = mm
	}
	mm[method] = h
}

func (c *Client) lookupHandler(path, method string) Handler {
	c.handlersMu.RLock()
	defer c.handlersMu.RUnlock()
	// Exact path first
	if mm := c.handlers[path]; mm != nil {
		if h := mm[method]; h != nil {
			return h
		}
		if h := mm["*"]; h != nil {
			return h
		}
	}
	// Parameterized trailing {param}
	for base, mm := range c.handlers {
		if !strings.Contains(base, "{") {
			continue
		}
		// Convert base like "/x/{id}" to prefix "/x/"
		i := strings.Index(base, "{")
		prefix := base[:i]
		if !strings.HasSuffix(prefix, "/") {
			prefix += "/"
		}
		if strings.HasPrefix(path, prefix) {
			if h := mm[method]; h != nil {
				return h
			}
			if h := mm["*"]; h != nil {
				return h
			}
		}
	}
	return nil
}

// Helpers
func (c *Client) send(v any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.enc.Encode(v)
}

func (c *Client) writeError(id json.RawMessage, code int, msg string) {
	_ = c.send(map[string]any{"jsonrpc": "2.0", "id": id, "error": rpcError{Code: code, Message: msg}})
}

// Convenience helpers

// Text returns a simple text/plain response.
func Text(status int, body string) *HTTPResponse {
	if status == 0 {
		status = http.StatusOK
	}
	return &HTTPResponse{
		Status:     status,
		Headers:    map[string][]string{"Content-Type": {"text/plain"}},
		BodyBase64: base64.StdEncoding.EncodeToString([]byte(body)),
	}
}

// JSON returns application/json response for any marshaled value.
func JSON(status int, v any) *HTTPResponse {
	if status == 0 {
		status = http.StatusOK
	}
	b, _ := json.Marshal(v)
	return &HTTPResponse{
		Status:     status,
		Headers:    map[string][]string{"Content-Type": {"application/json"}},
		BodyBase64: base64.StdEncoding.EncodeToString(b),
	}
}

func stringTrimQuotes(b json.RawMessage) string { var s string; _ = json.Unmarshal(b, &s); return s }

// RunMux is a convenience that runs the SDK using a Mux definition.
func (c *Client) RunMux(m *Mux) error {
	if m == nil {
		m = NewMux()
	}
	eps := m.endpoints()
	return c.Run(eps...)
}

// ManagerClient provides direct HTTP access to the management server.
type ManagerClient struct {
	baseURL string
	http    *http.Client
}

// Manager returns a client bound to the management server based on env vars.
// It reads BANYAN_MGMT_URL; if not set, returns nil.
func (c *Client) Manager() *ManagerClient {
	base := os.Getenv("BANYAN_MGMT_URL")
	if base == "" {
		return nil
	}
	return &ManagerClient{baseURL: strings.TrimRight(base, "/"), http: &http.Client{Timeout: 30 * time.Second}}
}

// Get issues a GET request to the management server with a relative path.
func (m *ManagerClient) Get(path string) (*http.Response, error) {
	if m == nil {
		return nil, errors.New("manager client not configured")
	}
	url := m.baseURL + "/" + strings.TrimLeft(path, "/")
	return m.http.Get(url)
}

// PostJSON issues a POST with JSON body to the management server with a relative path.
func (m *ManagerClient) PostJSON(path string, body any) (*http.Response, error) {
	if m == nil {
		return nil, errors.New("manager client not configured")
	}
	b, _ := json.Marshal(body)
	url := m.baseURL + "/" + strings.TrimLeft(path, "/")
	req, _ := http.NewRequest(http.MethodPost, url, strings.NewReader(string(b)))
	req.Header.Set("Content-Type", "application/json")
	return m.http.Do(req)
}

// Alias resolver API

// AddAliasResolver registers a function that can resolve an alias to a file path.
// The first resolver that returns a non-empty path with nil error wins.
func (c *Client) AddAliasResolver(fn func(alias string) (string, error)) {
	if fn == nil {
		return
	}
	c.aliasMu.Lock()
	c.resolvers = append(c.resolvers, fn)
	c.aliasMu.Unlock()
}

// AddAliasResolverEx registers a resolver that can return a file path or raw JSON.
// If both are returned, the path takes precedence at the host when both are provided.
func (c *Client) AddAliasResolverEx(fn func(alias string) (path string, json string, err error)) {
	if fn == nil {
		return
	}
	c.aliasMu.Lock()
	c.resolversEx = append(c.resolversEx, fn)
	c.aliasMu.Unlock()
}

func (c *Client) hasAliasResolvers() bool {
	c.aliasMu.RLock()
	n := len(c.resolvers)
	c.aliasMu.RUnlock()
	return n > 0
}

func (c *Client) aliasResolveHandler() Handler {
	type aliasResp struct {
		Alias string `json:"alias"`
		Path  string `json:"path,omitempty"`
		JSON  string `json:"json,omitempty"`
		Error string `json:"error,omitempty"`
	}
	return func(r *HTTPRequest) (*HTTPResponse, error) {
		alias := ""
		if r.PathParams != nil {
			alias = r.PathParams["alias"]
		}
		if alias == "" {
			return JSON(http.StatusBadRequest, aliasResp{Error: "missing alias"}), nil
		}
		c.aliasMu.RLock()
		resolvers := make([]func(string) (string, error), len(c.resolvers))
		copy(resolvers, c.resolvers)
		resolversEx := make([]func(string) (string, string, error), len(c.resolversEx))
		copy(resolversEx, c.resolversEx)
		c.aliasMu.RUnlock()
		var lastErr error
		// Try extended resolvers first
		for _, fn := range resolversEx {
			p, raw, err := fn(alias)
			if err == nil && (p != "" || raw != "") {
				return JSON(http.StatusOK, aliasResp{Alias: alias, Path: p, JSON: raw}), nil
			}
			if err != nil {
				lastErr = err
			}
		}
		for _, fn := range resolvers {
			p, err := fn(alias)
			if err == nil && p != "" {
				return JSON(http.StatusOK, aliasResp{Alias: alias, Path: p}), nil
			}
			if err != nil {
				lastErr = err
			}
		}
		msg := "not found"
		if lastErr != nil {
			msg = lastErr.Error()
		}
		return JSON(http.StatusNotFound, aliasResp{Alias: alias, Error: msg}), nil
	}
}

// runAliasResolvers tries all registered resolvers and returns the first non-empty
// result. Extended resolvers are tried before legacy path-only resolvers.
func (c *Client) runAliasResolvers(alias string) (path string, rawJSON string, err error) {
	c.aliasMu.RLock()
	resolversEx := make([]func(string) (string, string, error), len(c.resolversEx))
	copy(resolversEx, c.resolversEx)
	resolvers := make([]func(string) (string, error), len(c.resolvers))
	copy(resolvers, c.resolvers)
	c.aliasMu.RUnlock()
	for _, fn := range resolversEx {
		p, raw, e := fn(alias)
		if e == nil && (p != "" || raw != "") {
			return p, raw, nil
		}
		if e != nil {
			err = e
		}
	}
	for _, fn := range resolvers {
		p, e := fn(alias)
		if e == nil && p != "" {
			return p, "", nil
		}
		if e != nil {
			err = e
		}
	}
	if err == nil {
		err = errors.New("alias not resolved")
	}
	return "", "", err
}

// Disclose sends public-facing addon information to the host so it can be included in greetings.
// Name is required; if empty, the disclosure will be ignored by the host.
func (c *Client) Disclose(name, version string, info map[string]interface{}) error {
	if strings.TrimSpace(name) == "" {
		return errors.New("name is required for disclosure")
	}
	id := fmt.Sprintf("disclose-%d", time.Now().UnixNano())
	return c.send(map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  "addon_disclose",
		"params": map[string]any{
			"name":    name,
			"version": version,
			"info":    info,
		},
	})
}
