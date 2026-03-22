// Package addon provides lifecycle management for external addon processes.
// It handles spawning addon executables, JSON-RPC 2.0 communication over stdin/stdout,
// HTTP endpoint registration and forwarding, and alias resolution across multiple addons.
//
// The Manager loads addon configurations from an addons.json file and starts each
// addon as a subprocess. Addons communicate with the host via JSON-RPC, registering
// HTTP endpoints that can be mounted on local (management) or remote (p2p) servers.
//
// Example configuration (addons.json):
//
//	{
//	  "addons": [
//	    {"name": "echo", "exec": "./echo/echo-addon", "args": ["--foo", "bar"]}
//	  ]
//	}
package addon

import (
	"bufio"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"banyan/types"
)

type addonsFile struct {
	Addons []addonEntry `json:"addons"`
}

type addonEntry struct {
	Name string   `json:"name"`
	Exec string   `json:"exec"`
	Args []string `json:"args,omitempty"`
}

// Manager coordinates the lifecycle and communication of addon processes.
// It spawns addon executables, handles JSON-RPC communication, registers HTTP
// endpoints, and provides alias resolution across all running addons.
type Manager struct {
	ctx       context.Context
	cancel    context.CancelFunc
	logf      func(format string, v ...interface{})
	mgmtMount func(path string, h func(http.ResponseWriter, *http.Request))
	p2pMount  func(path string, h func(http.ResponseWriter, *http.Request))

	procs   map[string]*addonProcess
	procsMu sync.RWMutex

	addonsDirOverride string
	extraEnv          map[string]string

	// addon disclosures exposed in greetings
	disclosures   map[string]types.AddonDisclosure // key: addon process name
	disclosuresMu sync.RWMutex

	// registered HTTP handlers to avoid duplicate path registration
	registeredHandlers map[string]*multiMethodHandler
}

// multiMethodHandler handles multiple HTTP methods for the same path.
// It dispatches incoming requests to the appropriate addon based on the HTTP method.
type multiMethodHandler struct {
	basePath  string
	paramName string
	methods   map[string]string // method -> addonName
	mu        sync.RWMutex
	manager   *Manager
}

func (h *multiMethodHandler) addMethod(method, addonName string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.methods == nil {
		h.methods = make(map[string]string)
	}
	h.methods[method] = addonName
}

func (h *multiMethodHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.RLock()
	addonName, exists := h.methods[r.Method]
	h.mu.RUnlock()

	if !exists {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}

	// Use the existing handler logic
	handler := h.manager.makeHTTPHandlerWithParams(addonName, r.Method, h.basePath, h.paramName)
	handler(w, r)
}

var globalManager *Manager

// SetGlobalManager sets the process-wide addon manager reference for host components to access.
func SetGlobalManager(m *Manager) {
	globalManager = m
}

// GetGlobalManager returns the process-wide addon manager reference if set.
func GetGlobalManager() *Manager {
	return globalManager
}

type addonProcess struct {
	name      string
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	stdout    io.ReadCloser
	stderr    io.ReadCloser
	enc       *json.Encoder
	dec       *json.Decoder
	encMu     sync.Mutex
	pending   map[string]chan rpcResponse
	pendingMu sync.Mutex
}

// JSON-RPC 2.0 minimal

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"` // string or number or null
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcResponse struct {
	JSONRPC string           `json:"jsonrpc"`
	ID      json.RawMessage  `json:"id"`
	Result  *json.RawMessage `json:"result,omitempty"`
	Error   *rpcError        `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// HTTP payloads to/from addon

type httpForwardRequest struct {
	Method     string              `json:"method"`
	Path       string              `json:"path"`
	RawQuery   string              `json:"raw_query,omitempty"`
	Headers    map[string][]string `json:"headers,omitempty"`
	BodyBase64 string              `json:"body_base64,omitempty"`
	PathParams map[string]string   `json:"path_params,omitempty"`
}

type httpForwardResponse struct {
	Status     int                 `json:"status"`
	Headers    map[string][]string `json:"headers,omitempty"`
	BodyBase64 string              `json:"body_base64,omitempty"`
}

type registerEndpointParams struct {
	Kind   string `json:"kind"`   // "local" or "remote"
	Method string `json:"method"` // GET/POST/etc
	Path   string `json:"path"`   // supports trailing {param}: e.g. /addons/x/{slug}
}

// NewManager creates a new addon Manager with the given parent context and logger.
// If logger is nil, a default logger using fmt.Printf is used.
func NewManager(parent context.Context, logger func(format string, v ...interface{})) *Manager {
	ctx, cancel := context.WithCancel(parent)
	if logger == nil {
		logger = func(format string, v ...interface{}) { fmt.Printf(format+"\n", v...) }
	}
	return &Manager{
		ctx:         ctx,
		cancel:      cancel,
		logf:        logger,
		procs:       make(map[string]*addonProcess),
		extraEnv:    make(map[string]string),
		disclosures: make(map[string]types.AddonDisclosure),
	}
}

// WithMounts configures the mount functions for local (management) and remote (p2p) HTTP endpoints.
// The mgmt function mounts handlers on the management server; p2p mounts on the libp2p server.
func (m *Manager) WithMounts(mgmt func(path string, h func(http.ResponseWriter, *http.Request)), p2p func(path string, h func(http.ResponseWriter, *http.Request))) *Manager {
	m.mgmtMount = mgmt
	m.p2pMount = p2p
	return m
}

// WithDirectory sets a custom addons directory location, overriding the default search path.
func (m *Manager) WithDirectory(dir string) *Manager {
	m.addonsDirOverride = dir
	return m
}

// WithEnv adds an environment variable that will be passed to all spawned addon processes.
func (m *Manager) WithEnv(key, value string) *Manager {
	if m.extraEnv == nil {
		m.extraEnv = make(map[string]string)
	}
	m.extraEnv[key] = value
	return m
}

// WithManagerURL sets the management server base URL, exposed to addons via BANYAN_MGMT_URL.
func (m *Manager) WithManagerURL(url string) *Manager {
	return m.WithEnv("BANYAN_MGMT_URL", url)
}

// LoadAndStart reads addons.json from the configured directory and starts all addon processes.
// Directory priority: WithDirectory override > BANYAN_ADDONS_DIR env > executable's directory/addons.
func (m *Manager) LoadAndStart() error {
	addonsDir, err := m.effectiveAddonsDir()
	if err != nil {
		return err
	}
	cfgPath := filepath.Join(addonsDir, "addons.json")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			m.logf("No addons.json found at %s; skipping addons", cfgPath)
			return nil
		}
		return fmt.Errorf("read addons.json: %w", err)
	}
	data = stripUTF8BOM(data)
	var af addonsFile
	if err := json.Unmarshal(data, &af); err != nil {
		return fmt.Errorf("parse addons.json: %w", err)
	}
	for _, ent := range af.Addons {
		if err := m.startAddon(addonsDir, ent); err != nil {
			m.logf("failed to start addon %s: %v", ent.Name, err)
		}
	}
	return nil
}

func (m *Manager) effectiveAddonsDir() (string, error) {
	if m.addonsDirOverride != "" {
		return m.addonsDirOverride, nil
	}
	if v := os.Getenv("BANYAN_ADDONS_DIR"); v != "" {
		return v, nil
	}
	return defaultAddonsDir()
}

func defaultAddonsDir() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	base := filepath.Dir(exe)
	return filepath.Join(base, "addons"), nil
}

func (m *Manager) startAddon(baseDir string, ent addonEntry) error {
	if ent.Name == "" {
		return errors.New("addon entry missing name")
	}
	if ent.Exec == "" {
		return fmt.Errorf("addon %s missing exec path", ent.Name)
	}
	execPath := ent.Exec
	if !filepath.IsAbs(execPath) {
		execPath = filepath.Join(baseDir, execPath)
	}
	cmd := exec.CommandContext(m.ctx, execPath, ent.Args...)
	// Inherit minimal environment
	cmd.Env = os.Environ()
	// Append extra env vars
	if len(m.extraEnv) > 0 {
		for k, v := range m.extraEnv {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start addon %s: %w", ent.Name, err)
	}

	proc := &addonProcess{
		name:    ent.Name,
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		enc:     json.NewEncoder(stdin),
		dec:     json.NewDecoder(bufio.NewReader(stdout)),
		pending: make(map[string]chan rpcResponse),
	}
	m.procsMu.Lock()
	m.procs[ent.Name] = proc
	m.procsMu.Unlock()

	go m.logStderr(ent.Name, stderr)
	go m.runReader(proc)
	go m.handleInit(ent.Name, proc)
	return nil
}

func (m *Manager) logStderr(name string, r io.Reader) {
	s := bufio.NewScanner(r)
	for s.Scan() {
		m.logf("[addon:%s] %s", name, s.Text())
	}
}

// handleInit notifies the addon of host capabilities and awaits registrations.
func (m *Manager) handleInit(name string, _ *addonProcess) {
	_ = m.sendNotify(name, "host_ready", map[string]interface{}{
		"version":  "1",
		"platform": runtime.GOOS + "/" + runtime.GOARCH,
	})
}

// runReader processes all inbound messages from an addon (responses or requests)
func (m *Manager) runReader(p *addonProcess) {
	dec := p.dec
	for {
		var raw map[string]json.RawMessage
		if err := dec.Decode(&raw); err != nil {
			if err == io.EOF || errors.Is(err, io.ErrClosedPipe) {
				return
			}
			m.logf("addon %s decode error: %v", p.name, err)
			return
		}
		// Distinguish request vs response
		if _, hasMethod := raw["method"]; hasMethod {
			var req rpcRequest
			_ = json.Unmarshal(mustRaw(raw, "jsonrpc"), &req.JSONRPC)
			req.ID = raw["id"]
			req.Method = stringTrimQuotes(raw["method"]) // cheap parse
			req.Params = raw["params"]
			m.handleAddonRequest(p, &req)
			continue
		}
		// Response path
		var resp rpcResponse
		_ = json.Unmarshal(mustRaw(raw, "jsonrpc"), &resp.JSONRPC)
		resp.ID = raw["id"]
		if r, ok := raw["result"]; ok {
			rr := r
			resp.Result = &rr
		}
		if e, ok := raw["error"]; ok {
			var er rpcError
			_ = json.Unmarshal(e, &er)
			resp.Error = &er
		}
		m.deliverResponse(p, &resp)
	}
}

func (m *Manager) handleAddonRequest(p *addonProcess, req *rpcRequest) {
	switch req.Method {
	case "addon_register_endpoint":
		var rp registerEndpointParams
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &rp)
		}
		if rp.Path == "" || (rp.Kind != "local" && rp.Kind != "remote") || rp.Method == "" {
			m.writeError(p, req.ID, -32602, "invalid params")
			return
		}
		// Support trailing {param} placeholder
		basePath := rp.Path
		paramName := ""
		if i := strings.Index(basePath, "{"); i >= 0 && strings.HasSuffix(basePath, "}") {
			paramName = strings.TrimSuffix(basePath[i+1:], "}")
			paramName = strings.TrimSpace(paramName)
			basePath = basePath[:i]
			if !strings.HasSuffix(basePath, "/") {
				basePath += "/"
			}
		}

		// Check if we already have a handler registered for this path
		handlerKey := fmt.Sprintf("%s:%s", rp.Kind, basePath)
		if existingHandler, exists := m.registeredHandlers[handlerKey]; exists {
			// Add this method to the existing multi-method handler
			existingHandler.addMethod(rp.Method, p.name)
			m.logf("  ADDON %s -> %s %s %s (added to existing handler)", p.name, strings.ToUpper(rp.Kind), rp.Method, rp.Path)
		} else {
			// Create a new multi-method handler
			multiHandler := m.makeMultiMethodHandler(basePath, paramName)
			multiHandler.addMethod(rp.Method, p.name)

			// Store the handler for future method additions
			if m.registeredHandlers == nil {
				m.registeredHandlers = make(map[string]*multiMethodHandler)
			}
			m.registeredHandlers[handlerKey] = multiHandler

			// Mount the handler
			mount := func(mountFn func(string, func(http.ResponseWriter, *http.Request))) {
				mountFn(basePath, multiHandler.ServeHTTP)
			}
			if rp.Kind == "local" {
				if m.mgmtMount != nil {
					mount(m.mgmtMount)
				}
			} else {
				if m.p2pMount != nil {
					mount(m.p2pMount)
				}
			}
			m.logf("  ADDON %s -> %s %s %s", p.name, strings.ToUpper(rp.Kind), rp.Method, rp.Path)
		}
		m.writeResult(p, req.ID, map[string]any{"ok": true})
	case "addon_disclose":
		// Addon provides public-facing info to be included in greetings
		var in struct {
			Name    string                 `json:"name"`
			Version string                 `json:"version"`
			Info    map[string]interface{} `json:"info"`
		}
		if len(req.Params) > 0 {
			_ = json.Unmarshal(req.Params, &in)
		}
		if strings.TrimSpace(in.Name) == "" {
			// name is required to be exposed; if missing, ignore silently
			if len(req.ID) > 0 {
				m.writeResult(p, req.ID, map[string]any{"ok": false, "error": "name required"})
			}
			return
		}
		d := types.AddonDisclosure{
			Name:    in.Name,
			Version: strings.TrimSpace(in.Version),
			Info:    in.Info,
		}
		m.disclosuresMu.Lock()
		m.disclosures[p.name] = d
		m.disclosuresMu.Unlock()
		if len(req.ID) > 0 {
			m.writeResult(p, req.ID, map[string]any{"ok": true})
		}
	default:
		m.writeError(p, req.ID, -32601, "method not found")
	}
}

// DisclosureProvider returns a function that yields the current list of addon disclosures.
// Each addon may disclose its name, version, and metadata to be included in host greetings.
func (m *Manager) DisclosureProvider() func() []types.AddonDisclosure {
	return func() []types.AddonDisclosure {
		m.disclosuresMu.RLock()
		list := make([]types.AddonDisclosure, 0, len(m.disclosures))
		for _, v := range m.disclosures {
			if strings.TrimSpace(v.Name) == "" {
				continue
			}
			list = append(list, v)
		}
		m.disclosuresMu.RUnlock()
		return list
	}
}

func (m *Manager) makeHTTPHandlerWithParams(addonName, expectMethod, basePath, paramName string) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		if expectMethod != "*" && r.Method != expectMethod {
			w.WriteHeader(http.StatusMethodNotAllowed)
			return
		}
		var bodyB64 string
		if r.Body != nil {
			b, _ := io.ReadAll(r.Body)
			if len(b) > 0 {
				bodyB64 = base64.StdEncoding.EncodeToString(b)
			}
		}
		pp := map[string]string{}
		if paramName != "" {
			path := r.URL.Path
			if strings.HasPrefix(path, basePath) {
				rest := strings.TrimPrefix(path, basePath)
				seg := rest
				if j := strings.Index(seg, "/"); j >= 0 {
					seg = seg[:j]
				}
				pp[paramName] = seg
			}
		}
		params := httpForwardRequest{
			Method:     r.Method,
			Path:       r.URL.Path,
			RawQuery:   r.URL.RawQuery,
			Headers:    r.Header,
			BodyBase64: bodyB64,
			PathParams: pp,
		}
		var resp httpForwardResponse
		if err := m.call(addonName, "addon_handle_http", params, &resp); err != nil {
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(err.Error()))
			return
		}
		for k, vv := range resp.Headers {
			for _, v := range vv {
				w.Header().Add(k, v)
			}
		}
		if resp.Status == 0 {
			resp.Status = 200
		}
		w.WriteHeader(resp.Status)
		if resp.BodyBase64 != "" {
			b, _ := base64.StdEncoding.DecodeString(resp.BodyBase64)
			_, _ = w.Write(b)
		}
	}
}

func (m *Manager) makeHTTPHandler(addonName string, expectMethod string) func(http.ResponseWriter, *http.Request) {
	return m.makeHTTPHandlerWithParams(addonName, expectMethod, "", "")
}

func (m *Manager) makeMultiMethodHandler(basePath, paramName string) *multiMethodHandler {
	return &multiMethodHandler{
		basePath:  basePath,
		paramName: paramName,
		manager:   m,
	}
}

// RPC helpers

func (m *Manager) sendNotify(name, method string, params interface{}) error {
	p := m.getProc(name)
	if p == nil {
		return fmt.Errorf("unknown addon: %s", name)
	}
	req := map[string]any{"jsonrpc": "2.0", "method": method, "params": params}
	p.encMu.Lock()
	defer p.encMu.Unlock()
	return p.enc.Encode(&req)
}

func (m *Manager) call(name, method string, params interface{}, out interface{}) error {
	p := m.getProc(name)
	if p == nil {
		return fmt.Errorf("unknown addon: %s", name)
	}
	id := fmt.Sprintf("c-%d", time.Now().UnixNano())
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	ch := make(chan rpcResponse, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()
	p.encMu.Lock()
	err := p.enc.Encode(&req)
	p.encMu.Unlock()
	if err != nil {
		return err
	}
	resp := <-ch
	p.pendingMu.Lock()
	delete(p.pending, id)
	p.pendingMu.Unlock()
	if resp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
	}
	if out != nil && resp.Result != nil {
		return json.Unmarshal(*resp.Result, out)
	}
	return nil
}

// callWithTimeout issues a JSON-RPC call to an addon and waits for a response or context cancellation.
// On timeout, the pending waiter is cleaned up and ctx.Err() is returned.
func (m *Manager) callWithTimeout(ctx context.Context, name, method string, params interface{}, out interface{}) error {
	p := m.getProc(name)
	if p == nil {
		return fmt.Errorf("unknown addon: %s", name)
	}
	id := fmt.Sprintf("c-%d", time.Now().UnixNano())
	req := map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}
	ch := make(chan rpcResponse, 1)
	p.pendingMu.Lock()
	p.pending[id] = ch
	p.pendingMu.Unlock()
	p.encMu.Lock()
	err := p.enc.Encode(&req)
	p.encMu.Unlock()
	if err != nil {
		// cleanup mapping
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return err
	}
	select {
	case resp := <-ch:
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		if resp.Error != nil {
			return fmt.Errorf("rpc error %d: %s", resp.Error.Code, resp.Error.Message)
		}
		if out != nil && resp.Result != nil {
			return json.Unmarshal(*resp.Result, out)
		}
		return nil
	case <-ctx.Done():
		p.pendingMu.Lock()
		delete(p.pending, id)
		p.pendingMu.Unlock()
		return ctx.Err()
	}
}

// ResolveAliasFirst queries all addons in parallel and returns the first successful alias resolution.
// Returns the resolved path or raw JSON string. Context cancellation aborts pending queries.
func (m *Manager) ResolveAliasFirst(ctx context.Context, alias string) (path string, rawJSON string, err error) {
	m.procsMu.RLock()
	names := make([]string, 0, len(m.procs))
	for name := range m.procs {
		names = append(names, name)
	}
	m.procsMu.RUnlock()
	if len(names) == 0 {
		return "", "", fmt.Errorf("no addons available")
	}
	type result struct {
		path string
		json string
		err  error
	}
	resCh := make(chan result, len(names))
	ctxAll, cancelAll := context.WithCancel(ctx)
	defer cancelAll()
	for _, name := range names {
		addonName := name
		go func() {
			// Each call should honor global ctx; if one returns, we cancel others.
			var out struct {
				Alias string `json:"alias"`
				Path  string `json:"path"`
				JSON  string `json:"json"`
			}
			e := m.callWithTimeout(ctxAll, addonName, "alias_resolve", map[string]string{"alias": alias}, &out)
			if e != nil {
				resCh <- result{err: e}
				return
			}
			if strings.TrimSpace(out.Path) != "" || strings.TrimSpace(out.JSON) != "" {
				resCh <- result{path: out.Path, json: out.JSON, err: nil}
				return
			}
			resCh <- result{err: fmt.Errorf("no result")}
		}()
	}
	var lastErr error
	for i := 0; i < len(names); i++ {
		select {
		case r := <-resCh:
			if r.err == nil && (r.path != "" || r.json != "") {
				cancelAll()
				return r.path, r.json, nil
			}
			if r.err != nil {
				lastErr = r.err
			}
		case <-ctxAll.Done():
			if lastErr == nil {
				lastErr = ctxAll.Err()
			}
			return "", "", lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("alias not resolved")
	}
	return "", "", lastErr
}

// ResolveAlias resolves an alias to a path or JSON, optionally from a specific addon.
// If desiredAddon is empty, all addons are queried in parallel and the first success is returned.
// Returns the addon name that produced the result for caller attribution.
func (m *Manager) ResolveAlias(ctx context.Context, alias string, desiredAddon string) (path string, rawJSON string, addonName string, err error) {
	m.procsMu.RLock()
	names := make([]string, 0, len(m.procs))
	for name := range m.procs {
		names = append(names, name)
	}
	m.procsMu.RUnlock()
	if len(names) == 0 {
		return "", "", "", fmt.Errorf("no addons available")
	}

	// If a specific addon is requested, query only that addon
	if strings.TrimSpace(desiredAddon) != "" {
		target := strings.TrimSpace(desiredAddon)
		// Ensure the addon exists
		exists := false
		for _, n := range names {
			if n == target {
				exists = true
				break
			}
		}
		if !exists {
			return "", "", "", fmt.Errorf("requested addon not found: %s", target)
		}
		var out struct {
			Alias string `json:"alias"`
			Path  string `json:"path"`
			JSON  string `json:"json"`
		}
		if e := m.callWithTimeout(ctx, target, "alias_resolve", map[string]string{"alias": alias}, &out); e != nil {
			return "", "", "", e
		}
		if strings.TrimSpace(out.Path) == "" && strings.TrimSpace(out.JSON) == "" {
			return "", "", "", fmt.Errorf("alias not resolved by addon: %s", target)
		}
		return out.Path, out.JSON, target, nil
	}

	// Otherwise, query all addons in parallel and return first successful result with its name
	type result struct {
		name string
		path string
		json string
		err  error
	}
	resCh := make(chan result, len(names))
	ctxAll, cancelAll := context.WithCancel(ctx)
	defer cancelAll()
	for _, name := range names {
		addonName := name
		go func() {
			var out struct {
				Alias string `json:"alias"`
				Path  string `json:"path"`
				JSON  string `json:"json"`
			}
			e := m.callWithTimeout(ctxAll, addonName, "alias_resolve", map[string]string{"alias": alias}, &out)
			if e != nil {
				resCh <- result{name: addonName, err: e}
				return
			}
			if strings.TrimSpace(out.Path) != "" || strings.TrimSpace(out.JSON) != "" {
				resCh <- result{name: addonName, path: out.Path, json: out.JSON, err: nil}
				return
			}
			resCh <- result{name: addonName, err: fmt.Errorf("no result")}
		}()
	}
	var lastErr error
	for i := 0; i < len(names); i++ {
		select {
		case r := <-resCh:
			if r.err == nil && (r.path != "" || r.json != "") {
				cancelAll()
				return r.path, r.json, r.name, nil
			}
			if r.err != nil {
				lastErr = r.err
			}
		case <-ctxAll.Done():
			if lastErr == nil {
				lastErr = ctxAll.Err()
			}
			return "", "", "", lastErr
		}
	}
	if lastErr == nil {
		lastErr = fmt.Errorf("alias not resolved")
	}
	return "", "", "", lastErr
}

func (m *Manager) deliverResponse(p *addonProcess, resp *rpcResponse) {
	var idStr string
	_ = json.Unmarshal(resp.ID, &idStr)
	p.pendingMu.Lock()
	ch := p.pending[idStr]
	p.pendingMu.Unlock()
	if ch != nil {
		ch <- *resp
	}
}

func (m *Manager) writeResult(p *addonProcess, id json.RawMessage, result interface{}) {
	msg := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "result": result}
	p.encMu.Lock()
	_ = p.enc.Encode(&msg)
	p.encMu.Unlock()
}

func (m *Manager) writeError(p *addonProcess, id json.RawMessage, code int, message string) {
	msg := map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(id), "error": rpcError{Code: code, Message: message}}
	p.encMu.Lock()
	_ = p.enc.Encode(&msg)
	p.encMu.Unlock()
}

func (m *Manager) getProc(name string) *addonProcess {
	m.procsMu.RLock()
	defer m.procsMu.RUnlock()
	return m.procs[name]
}

// helpers
func mustRaw(m map[string]json.RawMessage, k string) json.RawMessage {
	if v, ok := m[k]; ok {
		return v
	}
	return nil
}
func stringTrimQuotes(b json.RawMessage) string { var s string; _ = json.Unmarshal(b, &s); return s }
func stripUTF8BOM(b []byte) []byte {
	if len(b) >= 3 && b[0] == 0xEF && b[1] == 0xBB && b[2] == 0xBF {
		return b[3:]
	}
	return b
}
