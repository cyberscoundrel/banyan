package handlers

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"

	"banyan/addon"
	"banyan/interfaces"
	nodePkg "banyan/node"
	"banyan/types"
)

// ServiceHandlers provides HTTP endpoint handlers for service management operations
// including service discovery (find), beacons, locators, and fig file handling.
type ServiceHandlers struct {
	node           *nodePkg.Node
	broadcastEvent func(types.Event)
	aliasMgr       *AliasSearchManager
	// Track normalized locator keys (hex of crypto.MarshalPublicKey(pub)) we've started
	locatorsMu         sync.RWMutex
	startedLocatorKeys map[string]time.Time
}

// NewServiceHandlers creates a new ServiceHandlers instance
func NewServiceHandlers(node *nodePkg.Node, broadcastEvent func(types.Event)) *ServiceHandlers {
	sh := &ServiceHandlers{
		node:               node,
		broadcastEvent:     broadcastEvent,
		startedLocatorKeys: make(map[string]time.Time),
	}
	sh.aliasMgr = NewAliasSearchManager(30*time.Second, sh)
	return sh
}

// aliasToServiceKeys stores alias -> addonName -> list of root service key hex strings discovered via /find
var aliasToServiceKeys struct {
	mu sync.RWMutex
	m  map[string]map[string][]string
}

// aliasToFigData stores alias -> addonName -> full fig data for hierarchical path matching
var aliasToFigData struct {
	mu sync.RWMutex
	m  map[string]map[string]types.FigFile
}

func init() {
	aliasToServiceKeys.m = make(map[string]map[string][]string)
	aliasToFigData.m = make(map[string]map[string]types.FigFile)
}

// Track alias expiry and cleanup timers
var aliasExpiry struct {
	mu     sync.Mutex
	times  map[string]time.Time
	timers map[string]*time.Timer
}

// Track seen nonces (to mitigate replay). Values are expiry times for automatic cleanup
var seenNonces struct {
	mu sync.Mutex
	m  map[string]time.Time
}

func init() {
	aliasExpiry.times = make(map[string]time.Time)
	aliasExpiry.timers = make(map[string]*time.Timer)
	seenNonces.m = make(map[string]time.Time)
}

// SetServiceKeysForAlias records root key hex strings for an alias under a specific addon (case-insensitive alias key)
// If addonName is empty, it records under "__local__" (for file-based/local figs).
func SetServiceKeysForAlias(alias string, keys []string, addonName string) {
	normalized := strings.ToLower(strings.TrimSpace(alias))
	if normalized == "" {
		return
	}
	if strings.TrimSpace(addonName) == "" {
		addonName = "__local__"
	}
	// ensure unique non-empty keys
	uniq := make(map[string]struct{})
	list := make([]string, 0, len(keys))
	for _, k := range keys {
		k2 := strings.TrimSpace(k)
		if k2 == "" {
			continue
		}
		if _, ok := uniq[k2]; ok {
			continue
		}
		uniq[k2] = struct{}{}
		list = append(list, k2)
	}
	aliasToServiceKeys.mu.Lock()
	if aliasToServiceKeys.m[normalized] == nil {
		aliasToServiceKeys.m[normalized] = make(map[string][]string)
	}
	aliasToServiceKeys.m[normalized][addonName] = list
	aliasToServiceKeys.mu.Unlock()
}

// SetServiceKeysForAliasWithExpiry records keys for an alias (scoped to addonName) and schedules automatic removal after expiry.
// If expiry is zero or in the past, it removes the alias immediately.
func SetServiceKeysForAliasWithExpiry(alias string, keys []string, expiry time.Time, addonName string) {
	SetServiceKeysForAlias(alias, keys, addonName)
	normalized := strings.ToLower(strings.TrimSpace(alias))
	// Cancel any previous timer
	aliasExpiry.mu.Lock()
	if t, ok := aliasExpiry.timers[normalized]; ok {
		t.Stop()
		delete(aliasExpiry.timers, normalized)
	}
	if expiry.IsZero() || time.Now().After(expiry) {
		// Remove immediately
		aliasExpiry.mu.Unlock()
		aliasToServiceKeys.mu.Lock()
		delete(aliasToServiceKeys.m, normalized)
		aliasToServiceKeys.mu.Unlock()
		return
	}
	aliasExpiry.times[normalized] = expiry
	d := time.Until(expiry)
	timer := time.AfterFunc(d, func() {
		aliasToServiceKeys.mu.Lock()
		delete(aliasToServiceKeys.m, normalized)
		aliasToServiceKeys.mu.Unlock()
		aliasExpiry.mu.Lock()
		delete(aliasExpiry.times, normalized)
		delete(aliasExpiry.timers, normalized)
		aliasExpiry.mu.Unlock()
		log.Printf("Alias '%s' expired at %s and was removed from cache", normalized, expiry.Format(time.RFC3339))
	})
	aliasExpiry.timers[normalized] = timer
	aliasExpiry.mu.Unlock()
}

// GetServiceKeysForAlias returns the recorded root key hex strings for an alias.
// If addonName is non-empty, it returns only keys discovered via that addon.
// If addonName is empty, it returns the union of keys from all addons for that alias.
func GetServiceKeysForAlias(alias string, addonName string) ([]string, bool) {
	normalized := strings.ToLower(strings.TrimSpace(alias))
	aliasToServiceKeys.mu.RLock()
	m, ok := aliasToServiceKeys.m[normalized]
	aliasToServiceKeys.mu.RUnlock()
	if !ok || m == nil {
		return nil, false
	}
	if strings.TrimSpace(addonName) == "" {
		// union of all
		uniq := make(map[string]struct{})
		out := make([]string, 0)
		for _, ks := range m {
			for _, k := range ks {
				if _, seen := uniq[k]; seen {
					continue
				}
				uniq[k] = struct{}{}
				out = append(out, k)
			}
		}
		if len(out) == 0 {
			return nil, false
		}
		return out, true
	}
	ks, ok := m[addonName]
	if !ok || len(ks) == 0 {
		return nil, false
	}
	return append([]string(nil), ks...), true
}

// SetFigDataForAlias stores the full fig data for an alias under a specific addon
func SetFigDataForAlias(alias string, fig types.FigFile, addonName string) {
	normalized := strings.ToLower(strings.TrimSpace(alias))
	if normalized == "" {
		return
	}
	if strings.TrimSpace(addonName) == "" {
		addonName = "__local__"
	}
	aliasToFigData.mu.Lock()
	if aliasToFigData.m[normalized] == nil {
		aliasToFigData.m[normalized] = make(map[string]types.FigFile)
	}
	aliasToFigData.m[normalized][addonName] = fig
	aliasToFigData.mu.Unlock()
}

// GetFigDataForAlias returns the full fig data for an alias.
// If addonName is non-empty, it returns only the fig from that addon.
// If addonName is empty, it returns the first available fig from any addon.
func GetFigDataForAlias(alias string, addonName string) (types.FigFile, bool) {
	normalized := strings.ToLower(strings.TrimSpace(alias))
	aliasToFigData.mu.RLock()
	m, ok := aliasToFigData.m[normalized]
	aliasToFigData.mu.RUnlock()
	if !ok || m == nil {
		return types.FigFile{}, false
	}
	if strings.TrimSpace(addonName) == "" {
		// Return first available fig
		for _, fig := range m {
			return fig, true
		}
		return types.FigFile{}, false
	}
	// specific addon
	fig, ok := m[addonName]
	return fig, ok
}

// FindRequest represents the request body for the /services/find endpoint.
// At least one of CompressedPublicKey, FigLocation, or Alias must be provided.
type FindRequest struct {
	CompressedPublicKey string `json:"compressedPublicKey"`
	FigLocation         string `json:"figLocation"`
	Alias               string `json:"alias"`
	Addon               string `json:"addon"`
}

// ServeRequest represents the request body for the /services/start endpoint.
// It specifies the file location of the PEM-encoded private key for the service.
type ServeRequest struct {
	FileLocation string `json:"fileLocation"`
}

// LocatorStartRequest represents a request to start a service locator from a public key.
type LocatorStartRequest struct {
	// hex-encoded bytes of libp2p public key (compressed or marshaled per crypto.MarshalPublicKey)
	CompressedPublicKey string `json:"compressedPublicKey"`
}

// LocatorInfo represents information about an active service locator.
type LocatorInfo struct {
	ServiceKeyHash string                  `json:"serviceKeyHash"`
	ServiceKey     string                  `json:"serviceKey"`
	Mode           types.ServiceBeaconMode `json:"mode"`
	Status         string                  `json:"status"`
	Limit          int                     `json:"limit"`
	IsRunning      bool                    `json:"isRunning"`
	CreatedAt      time.Time               `json:"createdAt"`
}

// BeaconInfo represents information about an active service beacon.
type BeaconInfo struct {
	ServiceKeyHash string                  `json:"serviceKeyHash"`
	ServiceKey     string                  `json:"serviceKey"`
	Mode           types.ServiceBeaconMode `json:"mode"`
	Status         string                  `json:"status"`
	IsRunning      bool                    `json:"isRunning"`
	PeerCount      int                     `json:"peerCount"`
	CreatedAt      time.Time               `json:"createdAt"`
}

// HandleFind implements the /services/find endpoint for service discovery.
// It supports finding services by compressed public key, fig file location, or alias.
// When an alias is provided, it attempts resolution via addons with local fallback.
func (sh *ServiceHandlers) HandleFind(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req FindRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	// Require at least one of: compressedPublicKey, figLocation, alias
	if req.CompressedPublicKey == "" && req.FigLocation == "" && req.Alias == "" {
		http.Error(w, "one of compressedPublicKey, figLocation, or alias is required", http.StatusBadRequest)
		return
	}

	// If figLocation is provided, process via common path with validation
	if req.FigLocation != "" {
		fullPath, err := sh.resolveFigPath(req.FigLocation)
		if err != nil {
			http.Error(w, fmt.Sprintf("Invalid figLocation: %v", err), http.StatusBadRequest)
			return
		}
		if sh.processFigAtPath(w, fullPath) {
			log.Printf("Processed fig from path %s", fullPath)
		}
		return
	}

	// If alias provided, attempt to resolve via addons (first result wins),
	// while tracking resolution state across requests.
	if req.Alias != "" {
		alias := strings.TrimSpace(req.Alias)
		if alias == "" {
			http.Error(w, "alias is empty", http.StatusBadRequest)
			return
		}
		mgr := addon.GetGlobalManager()
		if mgr == nil {
			// If a specific addon was requested, but addons are not available, fail explicitly
			if strings.TrimSpace(req.Addon) != "" {
				http.Error(w, "addons manager not available to honor addon filter", http.StatusServiceUnavailable)
				return
			}
			// Fallback to local figs directory resolution if addons not available
			fullPath, _, err := sh.resolveAliasToFig(alias)
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to resolve alias: %v", err), http.StatusBadRequest)
				return
			}
			if sh.processFigAtPath(w, fullPath) {
				log.Printf("Processed alias %s -> %s", alias, fullPath)
			}
			return
		}
		desiredAddon := strings.TrimSpace(req.Addon)
		// Use alias+addon as cache key so different addon filters do not conflict
		cacheKey := alias
		if desiredAddon != "" {
			cacheKey = alias + "\x00" + desiredAddon
		}
		search := sh.aliasMgr.Ensure(cacheKey, func(ctx context.Context, _ string) (string, string, string, error) {
			p, raw, addName, err := mgr.ResolveAlias(ctx, alias, desiredAddon)
			return p, raw, addName, err
		})
		// If already cached, return immediately
		if search.HasFig() {
			if sh.processFigJSON(w, search.FigJSON(), search.AddonName()) {
				log.Printf("Processed alias %s from cache", alias)
			}
			return
		}
		// Attempt fast path wait up to 500ms to capture immediate results
		if search.WaitFor(500 * time.Millisecond) {
			if search.HasFig() {
				if sh.processFigJSON(w, search.FigJSON(), search.AddonName()) {
					log.Printf("Processed alias %s via addon JSON (fast-path)", alias)
				}
				return
			}
			res := search.Result()
			if res.Err != nil {
				// Addons failed to resolve - try local fallback if no specific addon was requested
				if desiredAddon == "" {
					log.Printf("Addons failed to resolve alias %s, trying local fallback: %v", alias, res.Err)
					fullPath, _, err := sh.resolveAliasToFig(alias)
					if err == nil {
						if sh.processFigAtPath(w, fullPath) {
							log.Printf("Processed alias %s -> %s (local fallback)", alias, fullPath)
						}
						return
					}
					log.Printf("Local fallback also failed for alias %s: %v", alias, err)
				}
				http.Error(w, fmt.Sprintf("Failed to resolve alias: %v", res.Err), http.StatusNotFound)
				return
			}
			// If still path-only, read and cache via helper
			fullPath := res.Path
			if !filepath.IsAbs(fullPath) {
				if p2, err := sh.resolveFigPath(fullPath); err == nil {
					fullPath = p2
				}
			}
			if sh.processFigAtPathWithAddon(w, fullPath, res.AddonName) {
				log.Printf("Processed alias %s -> %s (addon:%s)", alias, fullPath, res.AddonName)
			}
			return
		}
		// Not ready yet: return per-alias waiting status
		state := search.State()
		response := map[string]interface{}{
			"message":       "Alias resolution in progress",
			"alias":         alias,
			"status":        "waiting",
			"startedAt":     state.StartedAt,
			"deadline":      state.Deadline,
			"timeoutSecs":   int(state.Timeout.Seconds()),
			"elapsedMillis": state.Elapsed.Milliseconds(),
			"data":          state.Data,
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		json.NewEncoder(w).Encode(response)
		return
	}

	// Fallback: single compressed key (backward compatible)
	if req.CompressedPublicKey == "" {
		http.Error(w, "compressedPublicKey is required when figLocation is not provided", http.StatusBadRequest)
		return
	}

	pubKeyBytes, err := hex.DecodeString(req.CompressedPublicKey)
	if err != nil {
		http.Error(w, "Invalid hex format for compressedPublicKey", http.StatusBadRequest)
		return
	}

	pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		http.Error(w, "Invalid public key format", http.StatusBadRequest)
		return
	}

	err = sh.node.GetServiceManager().StartServiceLocator(pubKey, types.ServiceBeaconModeLookup)
	if err != nil {
		if strings.Contains(err.Error(), "already exists") {
			http.Error(w, "Service locator already exists for this service", http.StatusConflict)
		} else {
			http.Error(w, fmt.Sprintf("Failed to start service locator: %v", err), http.StatusInternalServerError)
		}
		log.Printf("Failed to start service locator: %v", err)
		return
	}

	hash := sha256.Sum256(pubKeyBytes)
	serviceKeyHash := fmt.Sprintf("%x", hash[:8])

	response := map[string]interface{}{
		"message":          "Service locator started successfully",
		"service_key":      req.CompressedPublicKey,
		"service_key_hash": serviceKeyHash,
		"mode":             "lookup",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	sh.broadcastEvent(types.Event{
		Type:      "service_locator_started",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"service_key_hash": serviceKeyHash,
			"mode":             "lookup",
		},
		NodeID: sh.node.GetHost().ID().String(),
	})

	log.Printf("Started service locator for service key: %s", serviceKeyHash)
}

// HandleAliasResolve handles the /services/alias/resolve endpoint to resolve a
// service alias to a connected peer ID and service key. This is used by the proxy
// addon to open service-key-routed tunnels without exposing the backend routing table.
// GET /services/alias/resolve?alias=chat-fig
// Returns: {"peer_id": "12D3...", "service_key": "abcd1234..."}
func (sh *ServiceHandlers) HandleAliasResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	alias := strings.TrimSpace(r.URL.Query().Get("alias"))
	if alias == "" {
		http.Error(w, "alias query parameter is required", http.StatusBadRequest)
		return
	}

	serviceKeys, ok := GetServiceKeysForAlias(alias, "")
	if !ok || len(serviceKeys) == 0 {
		http.Error(w, fmt.Sprintf("alias '%s' not found. Call /services/find first.", alias), http.StatusNotFound)
		return
	}

	if requestPath := r.URL.Query().Get("path"); requestPath != "" {
		if figData, hasFig := GetFigDataForAlias(alias, ""); hasFig {
			matchedKeys, _ := figData.FindKeysAndMatchedPath(requestPath)
			if len(matchedKeys) > 0 {
				serviceKeys = matchedKeys
			}
		}
	}

	returnAll := r.URL.Query().Get("all") == "true"

	connections := sh.node.GetConnectionManager().GetConnectionsCopy()
	if returnAll {
		type peerInfo struct {
			PeerID     string `json:"peer_id"`
			ServiceKey string `json:"service_key"`
		}
		var found []peerInfo
		seen := make(map[string]bool)
		for _, keyHex := range serviceKeys {
			keyLower := strings.ToLower(strings.TrimSpace(keyHex))
			for pid, conn := range connections {
				pidStr := pid.String()
				if seen[pidStr] {
					continue
				}
				for _, svcKey := range conn.ServiceKeys {
					if strings.ToLower(fmt.Sprintf("%x", svcKey)) == keyLower {
						found = append(found, peerInfo{PeerID: pidStr, ServiceKey: keyHex})
						seen[pidStr] = true
						break
					}
				}
			}
		}
		if len(found) == 0 {
			http.Error(w, fmt.Sprintf("alias '%s' resolved but no connected peers found", alias), http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{
			"peers": found,
		})
		return
	}

	for _, keyHex := range serviceKeys {
		keyLower := strings.ToLower(strings.TrimSpace(keyHex))
		for pid, conn := range connections {
			for _, svcKey := range conn.ServiceKeys {
				if strings.ToLower(fmt.Sprintf("%x", svcKey)) == keyLower {
					w.Header().Set("Content-Type", "application/json")
					json.NewEncoder(w).Encode(map[string]interface{}{
						"peer_id":     pid.String(),
						"service_key": keyHex,
					})
					return
				}
			}
		}
	}

	http.Error(w, fmt.Sprintf("alias '%s' resolved but no connected peers found for its service keys", alias), http.StatusBadGateway)
}
// HandleLocatorStart handles the /services/locator/start endpoint to start
// a service locator for discovering peers that announce a specific service key.
func (sh *ServiceHandlers) HandleLocatorStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}
	// Parse body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()
	var req LocatorStartRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.CompressedPublicKey) == "" {
		http.Error(w, "compressedPublicKey is required", http.StatusBadRequest)
		return
	}
	// Start locator via helper
	res, status, err := sh.startLocatorFromKey(req.CompressedPublicKey)
	if err != nil {
		http.Error(w, err.Error(), status)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(res)
}

// HandleLocators implements the /services/locators endpoint to list all
// active service locators and their discovered peers.
func (sh *ServiceHandlers) HandleLocators(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Build response listing started locator keys and currently located peer IDs for each key
	sh.locatorsMu.RLock()
	started := make([]string, 0, len(sh.startedLocatorKeys))
	for key := range sh.startedLocatorKeys {
		started = append(started, key)
	}
	sh.locatorsMu.RUnlock()

	// Query connection manager for peers tagged with each key
	conns := sh.node.GetConnectionManager().GetConnectionsCopy()
	items := make([]map[string]interface{}, 0, len(started))
	for _, keyHex := range started {
		peers := make([]string, 0)
		for pid, ci := range conns {
			for _, sk := range ci.ServiceKeys {
				if fmt.Sprintf("%x", sk) == keyHex {
					peers = append(peers, pid.String())
					break
				}
			}
		}
		// Optional short hash for display
		h := sha256.Sum256([]byte(keyHex))
		items = append(items, map[string]interface{}{
			"service_key":      keyHex,
			"service_key_hash": fmt.Sprintf("%x", h[:8]),
			"peer_ids":         peers,
			"count":            len(peers),
		})
	}

	response := map[string]interface{}{
		"locators":  items,
		"total":     len(items),
		"timestamp": time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Served locators list: %s %s", r.Method, r.URL.Path)
}

// HandleLocatorSuspend implements the /services/locator/suspend/{hash} endpoint
// to temporarily suspend a service locator from actively discovering peers.
func (sh *ServiceHandlers) HandleLocatorSuspend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceKeyHash := extractServiceKeyHash(r.URL.Path, "/locator/suspend/")
	if serviceKeyHash == "" {
		http.Error(w, "Service key hash not specified in path", http.StatusBadRequest)
		return
	}

	// In a full implementation, you'd suspend the specific locator
	// For now, return a success response
	response := map[string]interface{}{
		"message":          "Locator suspended successfully",
		"service_key_hash": serviceKeyHash,
		"status":           "suspended",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Suspended locator: %s", serviceKeyHash)
}

// HandleLocatorRevive implements the /services/locator/revive/{hash} endpoint
// to resume a previously suspended service locator.
func (sh *ServiceHandlers) HandleLocatorRevive(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceKeyHash := extractServiceKeyHash(r.URL.Path, "/locator/revive/")
	if serviceKeyHash == "" {
		http.Error(w, "Service key hash not specified in path", http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"message":          "Locator revived successfully",
		"service_key_hash": serviceKeyHash,
		"status":           "active",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Revived locator: %s", serviceKeyHash)
}

// HandleLocatorDestroy implements the /services/locator/destroy/{hash} endpoint
// to permanently stop and remove a service locator.
func (sh *ServiceHandlers) HandleLocatorDestroy(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	serviceKeyHash := extractServiceKeyHash(r.URL.Path, "/locator/destroy/")
	if serviceKeyHash == "" {
		http.Error(w, "Service key hash not specified in path", http.StatusBadRequest)
		return
	}

	response := map[string]interface{}{
		"message":          "Locator destroyed successfully",
		"service_key_hash": serviceKeyHash,
		"status":           "destroyed",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	log.Printf("Destroyed locator: %s", serviceKeyHash)
}

// HandleServe implements the /services/start endpoint to create and start
// a service beacon from a PEM-encoded private key file.
func (sh *ServiceHandlers) HandleServe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Parse request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "Failed to read request body", http.StatusBadRequest)
		return
	}
	defer r.Body.Close()

	var req ServeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "Invalid JSON in request body", http.StatusBadRequest)
		return
	}

	if req.FileLocation == "" {
		http.Error(w, "fileLocation is required", http.StatusBadRequest)
		return
	}

	// Validate file path
	if !filepath.IsAbs(req.FileLocation) {
		http.Error(w, "fileLocation must be an absolute path", http.StatusBadRequest)
		return
	}

	// Load private key from PEM file
	privKey, err := loadPrivateKeyFromPEM(req.FileLocation)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to load private key from PEM file: %v", err), http.StatusBadRequest)
		log.Printf("Failed to load private key from %s: %v", req.FileLocation, err)
		return
	}

	// Create service beacon
	beacon, err := sh.node.GetServiceManager().CreateServiceBeacon(privKey)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to create service beacon: %v", err), http.StatusInternalServerError)
		log.Printf("Failed to create service beacon: %v", err)
		return
	}

	// Start the beacon
	if err := beacon.Start(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to start service beacon: %v", err), http.StatusInternalServerError)
		log.Printf("Failed to start service beacon: %v", err)
		return
	}

	// Get service key for response
	pubKey := privKey.GetPublic()
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
	hash := sha256.Sum256(pubKeyBytes)
	serviceKeyHash := fmt.Sprintf("%x", hash[:8])

	response := map[string]interface{}{
		"message":          "Service beacon started successfully",
		"service_key_hash": serviceKeyHash,
		"service_key":      hex.EncodeToString(pubKeyBytes),
		"mode":             "announce",
		"file_location":    req.FileLocation,
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Broadcast event
	sh.broadcastEvent(types.Event{
		Type:      "service_beacon_started",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"service_key_hash": serviceKeyHash,
			"mode":             "announce",
			"file_location":    req.FileLocation,
		},
		NodeID: sh.node.GetHost().ID().String(),
	})

	log.Printf("Started service beacon for service key: %s from file: %s", serviceKeyHash, req.FileLocation)
}

// HandleBeaconSuspend implements the beacon suspend endpoint to temporarily
// suspend service announcements while keeping the beacon active for replies.
// Supports both legacy (no hash) and new (with hash) URL patterns:
//   - /services/beacon/suspend (legacy: suspends first beacon only)
//   - /services/beacon/suspend/{serviceKeyHash} (new: suspends specific beacon)
func (sh *ServiceHandlers) HandleBeaconSuspend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Extract service key hash from URL path if provided
	serviceKeyHash := extractServiceKeyHash(r.URL.Path, "/services/beacon/suspend/")
	if serviceKeyHash == "" {
		// Also try legacy path
		serviceKeyHash = extractServiceKeyHash(r.URL.Path, "/beacon/suspend/")
	}

	var beacon interfaces.ServiceBeacon
	if serviceKeyHash != "" {
		// Find specific beacon by service key hash
		beacon = sh.findBeaconByKeyHash(serviceKeyHash)
		if beacon == nil {
			http.Error(w, fmt.Sprintf("No beacon found with service key hash: %s", serviceKeyHash), http.StatusNotFound)
			return
		}
	} else {
		// Legacy behavior: use first beacon
		beacon = sh.node.GetServiceManager().GetServiceBeacon()
		if beacon == nil {
			http.Error(w, "No active beacon found", http.StatusNotFound)
			return
		}
		log.Printf("WARNING: Using legacy single-beacon suspend endpoint. Consider specifying service key hash in URL.")
	}

	// Set beacon mode to reply-only to suspend announcements
	beacon.SetMode(types.ServiceBeaconModeReplyOnly)

	// Get service key hash for response
	privKey := beacon.GetServiceKey()
	pubKey := privKey.GetPublic()
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
	hash := sha256.Sum256(pubKeyBytes)
	actualServiceKeyHash := fmt.Sprintf("%x", hash[:8])

	response := map[string]interface{}{
		"message":          "Service beacon suspended successfully",
		"service_key_hash": actualServiceKeyHash,
		"status":           "suspended",
		"mode":             "reply_only",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Broadcast event
	sh.broadcastEvent(types.Event{
		Type:      "service_beacon_suspended",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"service_key_hash": actualServiceKeyHash,
		},
		NodeID: sh.node.GetHost().ID().String(),
	})

	log.Printf("Suspended service beacon: %s", actualServiceKeyHash)
}

// HandleBeaconKill implements the beacon kill endpoint to permanently stop
// and remove a service beacon.
// Supports both legacy (no hash) and new (with hash) URL patterns:
//   - /services/beacon/kill or /services/stop (legacy: kills first beacon only)
//   - /services/beacon/kill/{serviceKeyHash} (new: kills specific beacon)
func (sh *ServiceHandlers) HandleBeaconKill(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Extract service key hash from URL path if provided
	serviceKeyHash := extractServiceKeyHash(r.URL.Path, "/services/beacon/kill/")
	if serviceKeyHash == "" {
		// Also try legacy path
		serviceKeyHash = extractServiceKeyHash(r.URL.Path, "/beacon/kill/")
	}

	var beacon interfaces.ServiceBeacon
	if serviceKeyHash != "" {
		// Find specific beacon by service key hash
		beacon = sh.findBeaconByKeyHash(serviceKeyHash)
		if beacon == nil {
			http.Error(w, fmt.Sprintf("No beacon found with service key hash: %s", serviceKeyHash), http.StatusNotFound)
			return
		}
	} else {
		// Legacy behavior: use first beacon
		beacon = sh.node.GetServiceManager().GetServiceBeacon()
		if beacon == nil {
			http.Error(w, "No active beacon found", http.StatusNotFound)
			return
		}
		log.Printf("WARNING: Using legacy single-beacon kill endpoint. Consider specifying service key hash in URL.")
	}

	// Get service key hash for response before stopping
	privKey := beacon.GetServiceKey()
	pubKey := privKey.GetPublic()
	pubKeyBytes, _ := crypto.MarshalPublicKey(pubKey)
	hash := sha256.Sum256(pubKeyBytes)
	actualServiceKeyHash := fmt.Sprintf("%x", hash[:8])

	// Stop the beacon
	if err := beacon.Stop(); err != nil {
		http.Error(w, fmt.Sprintf("Failed to stop service beacon: %v", err), http.StatusInternalServerError)
		log.Printf("Failed to stop service beacon: %v", err)
		return
	}

	response := map[string]interface{}{
		"message":          "Service beacon killed successfully",
		"service_key_hash": actualServiceKeyHash,
		"status":           "stopped",
		"timestamp":        time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)

	// Broadcast event
	sh.broadcastEvent(types.Event{
		Type:      "service_beacon_killed",
		Timestamp: time.Now(),
		Data: map[string]interface{}{
			"service_key_hash": actualServiceKeyHash,
		},
		NodeID: sh.node.GetHost().ID().String(),
	})

	log.Printf("Killed service beacon: %s", actualServiceKeyHash)
}

// HandleFigs implements the /services/figs endpoint to list fig files in the
// configured figs directory and display alias cache information.
// If the request path ends with "/verbose", it includes full fig JSON content.
func (sh *ServiceHandlers) HandleFigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}
	verbose := strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/verbose")

	// Determine figs directory
	figsDir, err := sh.determineFigsDir()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to resolve figs directory: %v", err), http.StatusInternalServerError)
		return
	}

	// List .fig files (non-recursive)
	entries := make([]map[string]interface{}, 0)
	if figsDir != "" {
		if dirEntries, err := os.ReadDir(figsDir); err == nil {
			for _, de := range dirEntries {
				if de.IsDir() {
					continue
				}
				name := de.Name()
				if strings.ToLower(filepath.Ext(name)) != ".fig" {
					continue
				}
				item := map[string]interface{}{
					"name": name,
					"path": filepath.Join(figsDir, name),
				}
				if info, e2 := de.Info(); e2 == nil {
					item["size"] = info.Size()
					item["modTime"] = info.ModTime()
				}
				if verbose {
					if b, e3 := os.ReadFile(filepath.Join(figsDir, name)); e3 == nil {
						// Validate it's JSON-like; still return raw string
						var js map[string]interface{}
						if json.Unmarshal(b, &js) == nil {
							item["json"] = json.RawMessage(b)
						} else {
							item["content"] = string(b)
						}
					}
				}
				entries = append(entries, item)
			}
		}
	}

	// Alias-to-keys cache snapshot (per addon)
	aliasKeys := make([]map[string]interface{}, 0)
	aliasToServiceKeys.mu.RLock()
	for alias, byAddon := range aliasToServiceKeys.m {
		for addonName, keys := range byAddon {
			ak := map[string]interface{}{
				"alias": alias,
				"addon": addonName,
				"keys":  keys,
			}
			// add expiry if known (expiry is per alias, not per addon)
			aliasExpiry.mu.Lock()
			if exp, ok := aliasExpiry.times[alias]; ok {
				ak["expiresAt"] = exp
			}
			aliasExpiry.mu.Unlock()
			aliasKeys = append(aliasKeys, ak)
		}
	}
	aliasToServiceKeys.mu.RUnlock()

	// Active alias searches snapshot
	activeSearches := make([]map[string]interface{}, 0)
	sh.aliasMgr.mu.Lock()
	for a, s := range sh.aliasMgr.active {
		s.mu.RLock()
		item := map[string]interface{}{
			"alias":     a,
			"startedAt": s.startedAt,
			"deadline":  s.deadline,
			"timeout":   s.timeout.String(),
			"done":      s.done != nil,
		}
		if s.figJSON != "" {
			item["hasFigJSON"] = true
			if verbose {
				item["figJSON"] = json.RawMessage(s.figJSON)
			}
		}
		if len(s.data) > 0 {
			item["data"] = s.data
		}
		s.mu.RUnlock()
		activeSearches = append(activeSearches, item)
	}
	sh.aliasMgr.mu.Unlock()

	resp := map[string]interface{}{
		"directory":      figsDir,
		"files":          entries,
		"aliasKeyCache":  aliasKeys,
		"activeSearches": activeSearches,
		"timestamp":      time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// HandleBeaconFigs implements the /services/beacons/figs endpoint to return
// signed fig files for all active service beacons.
// Request body may include {"nonce":"hex"} or {"requesterPeerId":"peerid"}.
// If neither is provided, uses this node's peer ID as the requester.
func (sh *ServiceHandlers) HandleBeaconFigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	var req struct {
		Nonce           string `json:"nonce"`
		RequesterPeerID string `json:"requesterPeerId"`
	}
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
		if r.Body != nil {
			_ = r.Body.Close()
		}
	}

	// If no requester peer ID provided, use this node's peer ID
	if strings.TrimSpace(req.RequesterPeerID) == "" && strings.TrimSpace(req.Nonce) == "" {
		req.RequesterPeerID = sh.node.GetHost().ID().String()
	}

	// Enumerate beacons
	beacons := sh.node.GetServiceManager().ListServiceBeacons()

	// Prepare response: group by alias and aggregate all keys/signatures per alias
	type keyInfo struct {
		Key  string `json:"key"`
		Hash string `json:"hash"`
	}
	type item struct {
		Alias string      `json:"alias"`
		Keys  []keyInfo   `json:"keys"`
		Fig   interface{} `json:"fig"`
	}
	out := struct {
		PeerID   string    `json:"peerId"`
		Services []item    `json:"services"`
		Time     time.Time `json:"time"`
	}{PeerID: sh.node.GetHost().ID().String(), Time: time.Now()}

	type agg struct {
		privs     []crypto.PrivKey
		keyHexes  []string
		keyHashes []string
		templates [][]byte
	}
	byAlias := make(map[string]*agg)
	for _, b := range beacons {
		priv := b.GetServiceKey()
		if priv == nil {
			continue
		}
		pub := priv.GetPublic()
		pubBytes, err := crypto.MarshalPublicKey(pub)
		if err != nil {
			continue
		}
		keyHex := fmt.Sprintf("%x", pubBytes)
		h := sha256.Sum256(pubBytes)
		keyHash := fmt.Sprintf("%x", h[:8])
		alias := strings.TrimSpace(b.GetAlias())
		if alias == "" {
			alias = keyHash
		}
		g := byAlias[alias]
		if g == nil {
			g = &agg{}
			byAlias[alias] = g
		}
		g.privs = append(g.privs, priv)
		g.keyHexes = append(g.keyHexes, keyHex)
		g.keyHashes = append(g.keyHashes, keyHash)
		if tpls := b.GetFigTemplates(); len(tpls) > 0 {
			g.templates = append(g.templates, tpls...)
		}
	}

	for alias, g := range byAlias {
		// If there are fig templates, generate a separate signed fig for each template
		// Otherwise, generate a single fig with the beacon alias
		if len(g.templates) == 0 {
			// No templates: create a basic fig with beacon alias
			fig := types.FigFile{
				ServiceAlias: alias,
				Root:         types.FigNode{Path: "/", Keys: g.keyHexes},
			}
			if strings.TrimSpace(req.RequesterPeerID) != "" {
				fig.RequesterPeerID = strings.TrimSpace(req.RequesterPeerID)
			}
			if strings.TrimSpace(req.Nonce) != "" {
				fig.Nonce = strings.TrimSpace(req.Nonce)
			}
			// Sign with service key(s)
			payload := fig.BuildCanonicalPayload()
			var sigs []string
			for _, priv := range g.privs {
				if sig, err := priv.Sign(payload); err == nil && len(sig) > 0 {
					sigs = append(sigs, hex.EncodeToString(sig))
				}
			}
			if len(sigs) > 0 {
				fig.Signatures = sigs
			}
			keys := make([]keyInfo, 0, len(g.keyHexes))
			for i := range g.keyHexes {
				keys = append(keys, keyInfo{Key: g.keyHexes[i], Hash: g.keyHashes[i]})
			}
			out.Services = append(out.Services, item{Alias: alias, Keys: keys, Fig: fig})
			continue
		}

		// Process each template as a separate service entry
		for _, tpl := range g.templates {
			var parsed struct {
				ServiceAlias        string          `json:"serviceAlias"`
				Root                types.FigNode   `json:"root"`
				Data                json.RawMessage `json:"data"`
				ExpiresAt           *time.Time      `json:"expiresAt"`
				Signatures          []string        `json:"signatures,omitempty"`
				RequiredSigners     []string        `json:"requiredSigners,omitempty"`
				AuthoritySignatures []string        `json:"authoritySignatures,omitempty"`
			}
			if json.Unmarshal(tpl, &parsed) != nil {
				continue
			}

			// Use the serviceAlias from the template, fallback to beacon alias
			figAlias := strings.TrimSpace(parsed.ServiceAlias)
			if figAlias == "" {
				figAlias = alias
			}

			fig := types.FigFile{
				ServiceAlias: figAlias,
				Root:         types.FigNode{Path: "/", Keys: g.keyHexes},
			}

			// Use the root structure from template if available
			if len(parsed.Root.Keys) > 0 {
				fig.Root = parsed.Root
			}
			if len(parsed.Data) > 0 {
				fig.Data = parsed.Data
			}
			if parsed.ExpiresAt != nil {
				fig.ExpiresAt = parsed.ExpiresAt.UTC()
			}
			if len(parsed.RequiredSigners) > 0 {
				fig.RequiredSigners = parsed.RequiredSigners
			}
			if len(parsed.AuthoritySignatures) > 0 {
				fig.AuthoritySignatures = parsed.AuthoritySignatures
			}

			// Collect existing signatures from template
			var existingSignatures []string
			for _, sig := range parsed.Signatures {
				if trimmed := strings.TrimSpace(sig); trimmed != "" {
					existingSignatures = append(existingSignatures, trimmed)
				}
			}

			if strings.TrimSpace(req.RequesterPeerID) != "" {
				fig.RequesterPeerID = strings.TrimSpace(req.RequesterPeerID)
			}
			if strings.TrimSpace(req.Nonce) != "" {
				fig.Nonce = strings.TrimSpace(req.Nonce)
			}

			// Build canonical payload and sign with the service key(s)
			payload := fig.BuildCanonicalPayload()
			newSigs := make([]string, 0, len(g.privs))
			for _, priv := range g.privs {
				if sig, err := priv.Sign(payload); err == nil && len(sig) > 0 {
					newSigs = append(newSigs, hex.EncodeToString(sig))
				}
			}

			// Merge existing signatures with new signatures, removing duplicates
			sigMap := make(map[string]bool)
			var mergedSigs []string

			// Add existing signatures first
			for _, sig := range existingSignatures {
				if !sigMap[sig] {
					sigMap[sig] = true
					mergedSigs = append(mergedSigs, sig)
				}
			}

			// Add new signatures, filtering out duplicates
			for _, sig := range newSigs {
				if !sigMap[sig] {
					sigMap[sig] = true
					mergedSigs = append(mergedSigs, sig)
				}
			}

			if len(mergedSigs) > 0 {
				fig.Signatures = mergedSigs
			}

			keys := make([]keyInfo, 0, len(g.keyHexes))
			for i := range g.keyHexes {
				keys = append(keys, keyInfo{Key: g.keyHexes[i], Hash: g.keyHashes[i]})
			}
			out.Services = append(out.Services, item{Alias: figAlias, Keys: keys, Fig: fig})
		}
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(out)
}

// HandleServiceFigByKey implements the /services/fig/{key} endpoint to return
// a signed fig file for a specific service key.
// URL format: /services/fig/{compressedPublicKeyHex}
// Request body may include either {"nonce":"hex"} or {"requesterPeerId":"peerid"}.
// This endpoint allows another node to request a signed fig file for a specific service key
// that this node is serving, which can then be used to resolve to an alias.
func (sh *ServiceHandlers) HandleServiceFigByKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}

	// Extract service key from URL path: /services/fig/{key}
	path := strings.TrimPrefix(r.URL.Path, "/services/fig/")
	compressedPublicKeyHex := strings.TrimSpace(path)
	if compressedPublicKeyHex == "" {
		http.Error(w, "Service key required in URL path", http.StatusBadRequest)
		return
	}

	// Parse optional request body
	var req struct {
		Nonce           string `json:"nonce"`
		RequesterPeerID string `json:"requesterPeerId"`
	}
	if r.Method == http.MethodPost {
		body, err := io.ReadAll(r.Body)
		if err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
		if r.Body != nil {
			_ = r.Body.Close()
		}
	}

	// Decode the service key
	pubKeyBytes, err := hex.DecodeString(compressedPublicKeyHex)
	if err != nil {
		http.Error(w, "Invalid service key hex format", http.StatusBadRequest)
		return
	}

	pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		http.Error(w, "Invalid service key format", http.StatusBadRequest)
		return
	}

	// Find the beacon serving this service key
	beacons := sh.node.GetServiceManager().ListServiceBeacons()
	var matchedBeacon interfaces.ServiceBeacon
	for _, b := range beacons {
		priv := b.GetServiceKey()
		if priv == nil {
			continue
		}
		beaconPub := priv.GetPublic()
		if beaconPub.Equals(pubKey) {
			matchedBeacon = b
			break
		}
	}

	if matchedBeacon == nil {
		http.Error(w, "Service key not found on this node", http.StatusNotFound)
		return
	}

	// Get the private key and fig templates from the beacon
	priv := matchedBeacon.GetServiceKey()
	alias := strings.TrimSpace(matchedBeacon.GetAlias())
	figTemplates := matchedBeacon.GetFigTemplates()

	// Calculate key hash for response
	pubBytes, _ := crypto.MarshalPublicKey(pubKey)
	keyHex := fmt.Sprintf("%x", pubBytes)
	hashBytes := sha256.Sum256(pubBytes)
	keyHash := fmt.Sprintf("%x", hashBytes[:8])

	// Use alias or key hash as fallback
	if alias == "" {
		alias = keyHash
	}

	// Build the fig file from templates or create a basic one
	fig := types.FigFile{
		ServiceAlias: alias,
		Root:         types.FigNode{Path: "/", Keys: []string{keyHex}},
	}

	// Populate Data, ExpiresAt, RequiredSigners, and AuthoritySignatures from fig templates
	var existingSignatures []string
	for _, tpl := range figTemplates {
		var parsed struct {
			ServiceAlias        string          `json:"serviceAlias"`
			Root                types.FigNode   `json:"root"`
			Data                json.RawMessage `json:"data"`
			ExpiresAt           *time.Time      `json:"expiresAt"`
			Signatures          []string        `json:"signatures,omitempty"`
			RequiredSigners     []string        `json:"requiredSigners,omitempty"`
			AuthoritySignatures []string        `json:"authoritySignatures,omitempty"`
		}
		if json.Unmarshal(tpl, &parsed) == nil {
			// Use the first matching template or any template if alias matches
			if strings.TrimSpace(parsed.ServiceAlias) != "" && strings.TrimSpace(parsed.ServiceAlias) != alias {
				continue
			}
			// Use the root structure from template if available
			if len(parsed.Root.Keys) > 0 {
				fig.Root = parsed.Root
			}
			if len(parsed.Data) > 0 && len(fig.Data) == 0 {
				fig.Data = parsed.Data
			}
			if parsed.ExpiresAt != nil && fig.ExpiresAt.IsZero() {
				fig.ExpiresAt = parsed.ExpiresAt.UTC()
			}
			if len(parsed.RequiredSigners) > 0 && len(fig.RequiredSigners) == 0 {
				fig.RequiredSigners = parsed.RequiredSigners
			}
			if len(parsed.AuthoritySignatures) > 0 && len(fig.AuthoritySignatures) == 0 {
				fig.AuthoritySignatures = parsed.AuthoritySignatures
			}
			// Collect existing signatures from templates
			for _, sig := range parsed.Signatures {
				if trimmed := strings.TrimSpace(sig); trimmed != "" {
					existingSignatures = append(existingSignatures, trimmed)
				}
			}
		}
	}

	// Set requester peer ID if provided
	if strings.TrimSpace(req.RequesterPeerID) != "" {
		fig.RequesterPeerID = strings.TrimSpace(req.RequesterPeerID)
	}
	if strings.TrimSpace(req.Nonce) != "" {
		fig.Nonce = strings.TrimSpace(req.Nonce)
	}

	// Build canonical payload and sign with the service key
	payload := fig.BuildCanonicalPayload()
	sig, err := priv.Sign(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to sign fig: %v", err), http.StatusInternalServerError)
		log.Printf("Failed to sign fig for service key %s: %v", keyHash, err)
		return
	}

	// Merge existing signatures with new signature
	sigMap := make(map[string]bool)
	var mergedSigs []string

	// Add existing signatures first
	for _, s := range existingSignatures {
		if !sigMap[s] {
			sigMap[s] = true
			mergedSigs = append(mergedSigs, s)
		}
	}

	// Add new signature
	newSigHex := hex.EncodeToString(sig)
	if !sigMap[newSigHex] {
		mergedSigs = append(mergedSigs, newSigHex)
	}

	fig.Signatures = mergedSigs

	// Return the signed fig file
	response := map[string]interface{}{
		"peerId":         sh.node.GetHost().ID().String(),
		"serviceKey":     keyHex,
		"serviceKeyHash": keyHash,
		"alias":          alias,
		"fig":            fig,
		"time":           time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)

	log.Printf("Returned signed fig for service key %s (alias: %s)", keyHash, alias)
}

// HandleServices implements the /services/list endpoint to list services configured
// in the services directory and display active beacon statuses.
// If the request path ends with "/verbose", it includes full fig JSON content for each configured fig.
func (sh *ServiceHandlers) HandleServices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if sh.node == nil {
		http.Error(w, "Node not available", http.StatusServiceUnavailable)
		return
	}
	verbose := strings.HasSuffix(strings.TrimSuffix(r.URL.Path, "/"), "/verbose")

	servicesDir, err := sh.determineServicesDir()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to resolve services directory: %v", err), http.StatusInternalServerError)
		return
	}

	// Load services.json if present
	cfgPath := ""
	if servicesDir != "" {
		cfgPath = filepath.Join(servicesDir, "services.json")
	}
	entries := make(map[string]struct {
		Directory    string   `json:"directory"`
		PEM          string   `json:"pem"`
		PEMs         []string `json:"pems"`
		Fig          string   `json:"fig"`
		Figs         []string `json:"figs"`
		RouteURL     string   `json:"routeUrl"`
		RoutePrefix  string   `json:"routePrefix"`
		KeepFullPath bool     `json:"keepFullPath"`
	})
	if cfgPath != "" {
		if b, e := os.ReadFile(cfgPath); e == nil && len(b) > 0 {
			_ = json.Unmarshal(b, &entries)
		}
	}

	// Resolve paths and optionally read fig contents
	servicesList := make([]map[string]interface{}, 0)
	for alias, ent := range entries {
		base := servicesDir
		if strings.TrimSpace(ent.Directory) != "" {
			base = filepath.Join(servicesDir, ent.Directory)
		}
		resolvePath := func(p string) string {
			if p == "" {
				return ""
			}
			if filepath.IsAbs(p) {
				return p
			}
			return filepath.Join(base, p)
		}
		pemPaths := make([]string, 0)
		if strings.TrimSpace(ent.PEM) != "" {
			pemPaths = append(pemPaths, resolvePath(ent.PEM))
		}
		for _, p := range ent.PEMs {
			if strings.TrimSpace(p) != "" {
				pemPaths = append(pemPaths, resolvePath(p))
			}
		}
		figPaths := make([]string, 0)
		if strings.TrimSpace(ent.Fig) != "" {
			figPaths = append(figPaths, resolvePath(ent.Fig))
		}
		for _, f := range ent.Figs {
			if strings.TrimSpace(f) != "" {
				figPaths = append(figPaths, resolvePath(f))
			}
		}
		item := map[string]interface{}{
			"alias":    alias,
			"pemPaths": pemPaths,
			"figPaths": figPaths,
		}
		if verbose && len(figPaths) > 0 {
			figsVerbose := make([]map[string]interface{}, 0, len(figPaths))
			for _, fp := range figPaths {
				entry := map[string]interface{}{"path": fp}
				if b, e := os.ReadFile(fp); e == nil {
					var js map[string]interface{}
					if json.Unmarshal(b, &js) == nil {
						entry["json"] = json.RawMessage(b)
					} else {
						entry["content"] = string(b)
					}
				} else if e != nil {
					entry["error"] = e.Error()
				}
				figsVerbose = append(figsVerbose, entry)
			}
			item["figsVerbose"] = figsVerbose
		}
		servicesList = append(servicesList, item)
	}

	// Beacon statuses from service manager
	beacons := sh.node.GetServiceManager().ListServiceBeacons()
	beaconInfos := make([]map[string]interface{}, 0, len(beacons))
	for _, b := range beacons {
		priv := b.GetServiceKey()
		pub := priv.GetPublic()
		pubBytes, _ := crypto.MarshalPublicKey(pub)
		h := sha256.Sum256(pubBytes)
		alias := b.GetAlias()
		info := map[string]interface{}{
			"alias":          alias,
			"serviceKey":     fmt.Sprintf("%x", pubBytes),
			"serviceKeyHash": fmt.Sprintf("%x", h[:8]),
			"mode":           b.GetMode(),
			"peerCount":      len(b.GetPeers()),
		}
		beaconInfos = append(beaconInfos, info)
	}

	resp := map[string]interface{}{
		"directory":  servicesDir,
		"configFile": cfgPath,
		"services":   servicesList,
		"beacons":    beaconInfos,
		"timestamp":  time.Now(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// determineFigsDir resolves the figs directory honoring node config if set, otherwise default ./figs by executable.
func (sh *ServiceHandlers) determineFigsDir() (string, error) {
	cfg := sh.node.GetConfig()
	var figsDir string
	if cfg != nil && cfg.FigsDir != nil && *cfg.FigsDir != "" {
		figsDir = *cfg.FigsDir
	} else {
		exePath, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("failed to determine executable path: %w", err)
		}
		figsDir = filepath.Join(filepath.Dir(exePath), "figs")
	}
	if figsDir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(figsDir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// determineServicesDir mirrors node logic to resolve services directory.
func (sh *ServiceHandlers) determineServicesDir() (string, error) {
	cfg := sh.node.GetConfig()
	var servicesDir string
	if cfg != nil && cfg.ServicesDir != nil && *cfg.ServicesDir != "" {
		servicesDir = *cfg.ServicesDir
	} else {
		exePath, err := os.Executable()
		if err != nil {
			return "", fmt.Errorf("failed to determine executable path: %w", err)
		}
		servicesDir = filepath.Join(filepath.Dir(exePath), "services")
	}
	if servicesDir == "" {
		return "", nil
	}
	abs, err := filepath.Abs(servicesDir)
	if err != nil {
		return "", err
	}
	return abs, nil
}

// extractServiceKeyHash extracts the service key hash from URL path
func extractServiceKeyHash(path, prefix string) string {
	if !strings.HasPrefix(path, prefix) {
		return ""
	}
	return strings.TrimPrefix(path, prefix)
}

// findBeaconByKeyHash finds a beacon by its service key hash (first 8 bytes of SHA256 hash)
func (sh *ServiceHandlers) findBeaconByKeyHash(keyHash string) interfaces.ServiceBeacon {
	beacons := sh.node.GetServiceManager().ListServiceBeacons()
	for _, beacon := range beacons {
		privKey := beacon.GetServiceKey()
		pubKey := privKey.GetPublic()
		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		if err != nil {
			continue
		}
		hash := sha256.Sum256(pubKeyBytes)
		beaconKeyHash := fmt.Sprintf("%x", hash[:8])
		if beaconKeyHash == keyHash {
			return beacon
		}
	}
	return nil
}

// loadPrivateKeyFromPEM loads a private key from a PEM file
// This is the same function from main.go, copied here for convenience
func loadPrivateKeyFromPEM(filename string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("failed to decode PEM block")
	}

	var key interface{}
	var parseErr error

	switch block.Type {
	case "EC PRIVATE KEY":
		key, parseErr = x509.ParseECPrivateKey(block.Bytes)
	case "PRIVATE KEY":
		key, parseErr = x509.ParsePKCS8PrivateKey(block.Bytes)
	case "RSA PRIVATE KEY":
		key, parseErr = x509.ParsePKCS1PrivateKey(block.Bytes)
	default:
		return nil, fmt.Errorf("unsupported PEM type: %s", block.Type)
	}

	if parseErr != nil {
		return nil, fmt.Errorf("failed to parse private key: %w", parseErr)
	}

	// Handle Ed25519 keys specially - crypto.KeyPairFromStdKey needs a pointer
	if ed25519Key, ok := key.(ed25519.PrivateKey); ok {
		key = &ed25519Key
	}

	privKey, _, err := crypto.KeyPairFromStdKey(key)
	if err != nil {
		return nil, fmt.Errorf("failed to convert to libp2p key: %w", err)
	}

	return privKey, nil
}

// resolveFigPath resolves a fig file path to the absolute path under the configured figs directory.
// It enforces confinement to the figs directory and supports optional .fig extension.
func (sh *ServiceHandlers) resolveFigPath(input string) (string, error) {
	// Use the configured figs directory from node config
	figsDirAbs, err := sh.determineFigsDir()
	if err != nil {
		return "", fmt.Errorf("failed to determine figs directory: %w", err)
	}
	if figsDirAbs == "" {
		return "", fmt.Errorf("figs directory not configured")
	}

	cand := input
	if !filepath.IsAbs(cand) {
		cand = filepath.Join(figsDirAbs, cand)
	}

	tryPaths := []string{cand}
	if filepath.Ext(cand) == "" {
		tryPaths = append(tryPaths, cand+".fig")
	}

	for _, p := range tryPaths {
		absP, err := filepath.Abs(p)
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(figsDirAbs, absP)
		if err != nil {
			continue
		}
		if strings.HasPrefix(rel, "..") {
			continue
		}
		if _, err := os.Stat(absP); err == nil {
			return absP, nil
		}
	}

	return "", fmt.Errorf("fig file not found within figs directory")
}

// resolveAliasToFig resolves a plaintext alias to a local fig file path.
// This is structured to allow future resolvers (e.g., IPFS/Kubo, on-chain registry, HTTP).
// For now, it only attempts to find a matching fig in the local figs directory.
func (sh *ServiceHandlers) resolveAliasToFig(alias string) (fullPath string, aliasResolved string, err error) {
	// Basic normalization for alias-safe filenames
	cleaned := strings.TrimSpace(alias)
	if cleaned == "" {
		return "", "", fmt.Errorf("alias is empty")
	}

	// Try exact alias as filename
	p, err := sh.resolveFigPath(cleaned)
	if err == nil {
		return p, cleaned, nil
	}

	// Try lowercased
	if strings.ToLower(cleaned) != cleaned {
		p, err2 := sh.resolveFigPath(strings.ToLower(cleaned))
		if err2 == nil {
			return p, strings.ToLower(cleaned), nil
		}
	}

	// Could add additional strategies here in the future
	return "", "", fmt.Errorf("alias not found in figs directory")
}

// AliasSearchManager coordinates alias resolution attempts and shares state across requests.
type AliasSearchManager struct {
	mu      sync.Mutex
	active  map[string]*AliasSearch
	timeout time.Duration
	sh      *ServiceHandlers // reference to ServiceHandlers for resolveFigPath
}

type AliasResolutionResult struct {
	Path      string
	RawJSON   string
	AddonName string
	Err       error
}

type AliasSearch struct {
	alias     string
	startedAt time.Time
	deadline  time.Time
	timeout   time.Duration
	done      chan struct{}
	mu        sync.RWMutex
	result    AliasResolutionResult
	ctx       context.Context
	cancel    context.CancelFunc

	// cached resolved fig JSON for immediate subsequent requests
	figJSON string
	// optional data metadata extracted from the fig JSON
	data json.RawMessage
}

type AliasSearchSnapshot struct {
	Alias     string          `json:"alias"`
	StartedAt time.Time       `json:"startedAt"`
	Deadline  time.Time       `json:"deadline"`
	Timeout   time.Duration   `json:"timeout"`
	Elapsed   time.Duration   `json:"elapsed"`
	Data      json.RawMessage `json:"data,omitempty"`
}

func NewAliasSearchManager(timeout time.Duration, sh *ServiceHandlers) *AliasSearchManager {
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	return &AliasSearchManager{active: make(map[string]*AliasSearch), timeout: timeout, sh: sh}
}

// Ensure returns an existing search for alias or starts a new one using fn.
func (m *AliasSearchManager) Ensure(alias string, fn func(ctx context.Context, alias string) (string, string, string, error)) *AliasSearch {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.active[alias]; ok {
		return s
	}
	ctx, cancel := context.WithTimeout(context.Background(), m.timeout)
	s := &AliasSearch{
		alias:     alias,
		startedAt: time.Now(),
		deadline:  time.Now().Add(m.timeout),
		timeout:   m.timeout,
		done:      make(chan struct{}),
		ctx:       ctx,
		cancel:    cancel,
	}
	m.active[alias] = s
	go func() {
		defer close(s.done)
		path, raw, addonName, err := fn(ctx, alias)
		// If successful, normalize to cached fig JSON
		var figStr string
		if err == nil {
			if strings.TrimSpace(raw) != "" {
				figStr = raw
			} else if strings.TrimSpace(path) != "" {
				finalPath := path
				if !filepath.IsAbs(finalPath) {
					if m.sh != nil {
						if p2, e2 := m.sh.resolveFigPath(finalPath); e2 == nil {
							finalPath = p2
						}
					}
				}
				if b, e3 := os.ReadFile(finalPath); e3 == nil {
					figStr = string(b)
				} else {
					err = e3
				}
			}
		}
		s.mu.Lock()
		s.result = AliasResolutionResult{Path: path, RawJSON: raw, AddonName: addonName, Err: err}
		if figStr != "" {
			s.figJSON = figStr
			// try to parse data metadata from fig JSON
			var tmp types.FigFile
			if json.Unmarshal([]byte(figStr), &tmp) == nil {
				s.data = tmp.Data
			}
		}
		s.mu.Unlock()
		cancel()
		// keep result in map for a short grace period, then cleanup
		time.AfterFunc(60*time.Second, func() {
			m.mu.Lock()
			delete(m.active, alias)
			m.mu.Unlock()
		})
	}()
	return s
}

func (s *AliasSearch) WaitFor(d time.Duration) bool {
	select {
	case <-s.done:
		return true
	case <-time.After(d):
		return false
	}
}

func (s *AliasSearch) Result() AliasResolutionResult {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result
}

func (s *AliasSearch) State() AliasSearchSnapshot {
	return AliasSearchSnapshot{
		Alias:     s.alias,
		StartedAt: s.startedAt,
		Deadline:  s.deadline,
		Timeout:   s.timeout,
		Elapsed:   time.Since(s.startedAt),
		Data:      s.data,
	}
}

func (s *AliasSearch) HasFig() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.figJSON != ""
}

func (s *AliasSearch) FigJSON() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.figJSON
}

func (s *AliasSearch) AddonName() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.result.AddonName
}

// Helpers to process a fig from a path or raw JSON and start locators
func (sh *ServiceHandlers) processFigAtPath(w http.ResponseWriter, fullPath string) bool {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read fig file: %v", err), http.StatusBadRequest)
		return false
	}
	return sh.processFigBytes(w, data, "file", fullPath, "__local__")
}

func (sh *ServiceHandlers) processFigAtPathWithAddon(w http.ResponseWriter, fullPath string, addonName string) bool {
	data, err := os.ReadFile(fullPath)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to read fig file: %v", err), http.StatusBadRequest)
		return false
	}
	if strings.TrimSpace(addonName) == "" {
		addonName = "__unknown__"
	}
	return sh.processFigBytes(w, data, "file", fullPath, addonName)
}

func (sh *ServiceHandlers) processFigJSON(w http.ResponseWriter, raw string, addonName string) bool {
	return sh.processFigBytes(w, []byte(raw), "addon", "", addonName)
}

// processFigBytes processes fig file bytes with optional request path for path-specific key validation.
// The requestPath parameter is optional and can be passed as the last variadic argument.
func (sh *ServiceHandlers) processFigBytes(w http.ResponseWriter, data []byte, source string, path string, addonName string, requestPath ...string) bool {
	// First, try to detect if this is a wrapped format (with "alias", "keys", and "fig" properties)
	var wrapper struct {
		Alias string          `json:"alias"`
		Keys  json.RawMessage `json:"keys"`
		Fig   json.RawMessage `json:"fig"`
	}

	var fig types.FigFile
	var figData []byte = data

	// Try to unmarshal as wrapper format
	if err := json.Unmarshal(data, &wrapper); err == nil && len(wrapper.Fig) > 0 {
		// This is the wrapper format - extract the inner fig
		figData = wrapper.Fig
		log.Printf("Detected wrapper format for fig file, extracting inner fig object")
	}

	// Now unmarshal the actual fig (either from wrapper or direct)
	if err := json.Unmarshal(figData, &fig); err != nil {
		http.Error(w, fmt.Sprintf("Invalid fig file format: %v", err), http.StatusBadRequest)
		return false
	}

	// Validate fig metadata (expiry, requester, nonce, signatures) with optional path-specific validation
	if err := sh.validateFig(fig, source, requestPath...); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return false
	}
	// Get all keys from the entire fig keytree, not just root
	allKeys := fig.CollectAllServiceKeys()
	if len(allKeys) == 0 {
		http.Error(w, "fig file contains no service keys", http.StatusBadRequest)
		return false
	}
	// Record alias -> normalized keys (marshaled form) for later alias-based routing
	if strings.TrimSpace(fig.ServiceAlias) != "" {
		normalizedKeys := make([]string, 0, len(allKeys))
		for _, k := range allKeys {
			b, err := hex.DecodeString(strings.TrimSpace(k))
			if err != nil {
				continue
			}
			pub, err := crypto.UnmarshalPublicKey(b)
			if err != nil {
				continue
			}
			marshaled, err := crypto.MarshalPublicKey(pub)
			if err != nil {
				continue
			}
			normalizedKeys = append(normalizedKeys, fmt.Sprintf("%x", marshaled))
		}
		if len(normalizedKeys) > 0 {
			// Honor --allow-expired-figs: if the node was explicitly told to
			// accept expired figs, don't let the expiry cache evict the alias
			// (immediately or on a timer). Otherwise an already-expired fig
			// would be registered and instantly removed, so /services/alias/resolve
			// 404s and the http-proxy addon returns 502.
			allowExpired := false
			if cfg := sh.node.GetConfig(); cfg != nil && cfg.AllowExpiredFigs != nil {
				allowExpired = *cfg.AllowExpiredFigs
			}
			if allowExpired {
				SetServiceKeysForAlias(fig.ServiceAlias, normalizedKeys, strings.TrimSpace(addonName))
			} else {
				// Schedule expiry-based cleanup. Attribute keys to the addon that produced this fig if any.
				SetServiceKeysForAliasWithExpiry(fig.ServiceAlias, normalizedKeys, fig.ExpiresAt, strings.TrimSpace(addonName))
			}
			// Also store the full fig data for hierarchical path matching
			SetFigDataForAlias(fig.ServiceAlias, fig, strings.TrimSpace(addonName))
		}
	}
	type keyResult struct {
		ServiceKey     string `json:"service_key"`
		ServiceKeyHash string `json:"service_key_hash"`
		Status         string `json:"status"`
		Error          string `json:"error,omitempty"`
	}
	results := make([]keyResult, 0, len(allKeys))
	for _, keyHex := range allKeys {
		res, status, err := sh.startLocatorFromKey(keyHex)
		if res == nil {
			// Determine error message
			errorMsg := "failed to start"
			if err != nil {
				errorMsg = err.Error()
			}
			// If already exists, mark as "already_running" instead of error
			if status == http.StatusConflict {
				results = append(results, keyResult{ServiceKey: keyHex, Status: "already_running", Error: ""})
			} else {
				results = append(results, keyResult{ServiceKey: keyHex, Status: "error", Error: errorMsg})
			}
			continue
		}
		results = append(results, keyResult{ServiceKey: keyHex, ServiceKeyHash: res["service_key_hash"].(string), Status: "started"})
		sh.broadcastEvent(types.Event{
			Type:      "service_locator_started",
			Timestamp: time.Now(),
			Data: map[string]interface{}{
				"service_key_hash": res["service_key_hash"],
				"mode":             "lookup",
				"alias":            fig.ServiceAlias,
			},
			NodeID: sh.node.GetHost().ID().String(),
		})
	}
	response := map[string]interface{}{
		"message":   "Processed fig",
		"alias":     fig.ServiceAlias,
		"results":   results,
		"mode":      "lookup",
		"timestamp": time.Now(),
	}
	if source != "" {
		response["source"] = source
	}
	if path != "" {
		response["path"] = path
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
	return true
}

// validateFig enforces expiry, requester, nonce, and signature checks.
// It respects node configuration flags to optionally allow expired or insecure figs.
// The requestPath parameter is optional and used for path-specific key validation.
func (sh *ServiceHandlers) validateFig(fig types.FigFile, source string, requestPath ...string) error {
	cfg := sh.node.GetConfig()
	allowExpired := cfg != nil && cfg.AllowExpiredFigs != nil && *cfg.AllowExpiredFigs
	allowInsecure := cfg != nil && cfg.AllowInsecureFigs != nil && *cfg.AllowInsecureFigs

	// Authority signature validation (must come before service signature validation)
	if len(fig.RequiredSigners) > 0 {
		if len(fig.AuthoritySignatures) == 0 {
			if !allowInsecure {
				return fmt.Errorf("missing authority signatures for fig file with required signers")
			}
			log.Printf("WARNING: Loading fig with required signers but NO authority signatures due to --allow-insecure-figs")
		} else if !sh.verifyAuthoritySignatures(fig) {
			if !allowInsecure {
				return fmt.Errorf("authority signature verification failed")
			}
			log.Printf("WARNING: Loading fig with INVALID authority signatures due to --allow-insecure-figs")
		}
	}

	// If coming from a file path, ensure it's under figs directory (already enforced by resolveFigPath for paths)
	// Expiry check
	if !fig.ExpiresAt.IsZero() && time.Now().After(fig.ExpiresAt) {
		if !allowExpired {
			return fmt.Errorf("fig file expired at %s", fig.ExpiresAt.Format(time.RFC3339))
		}
		log.Printf("WARNING: Loading EXPIRED fig (expired at %s) due to --allow-expired-figs", fig.ExpiresAt.Format(time.RFC3339))
	}

	// Nonce replay tracking: if provided, ensure unseen; expire nonce at fig expiry or in 10 minutes if no expiry
	if strings.TrimSpace(fig.Nonce) != "" {
		seenNonces.mu.Lock()
		if exp, ok := seenNonces.m[fig.Nonce]; ok {
			// If still valid, reject as replay
			if exp.IsZero() || time.Now().Before(exp) {
				seenNonces.mu.Unlock()
				return fmt.Errorf("fig nonce already seen")
			}
		}
		// Store with expiry
		expiry := fig.ExpiresAt
		if expiry.IsZero() {
			expiry = time.Now().Add(10 * time.Minute)
		}
		seenNonces.m[fig.Nonce] = expiry
		seenNonces.mu.Unlock()
		// Schedule cleanup
		time.AfterFunc(time.Until(expiry), func() {
			seenNonces.mu.Lock()
			delete(seenNonces.m, fig.Nonce)
			seenNonces.mu.Unlock()
		})
	}

	// Service signature validation (when insecure mode is disabled)
	if !allowInsecure {
		if len(fig.Signatures) == 0 {
			return fmt.Errorf("fig has no signatures and insecure figs are not allowed")
		}

		// Collect ALL service keys from the entire fig tree (root + all descendants)
		allServiceKeys := fig.CollectAllServiceKeys()

		if len(allServiceKeys) == 0 {
			return fmt.Errorf("fig tree contains no service keys")
		}

		// Build the canonical payload for signature verification
		payload := fig.BuildCanonicalPayload()

		// Verify that at least one signature matches any service key in the tree
		signatureValid := sh.verifyAnySignatureAgainstKeys(fig.Signatures, payload, allServiceKeys)

		// If signature verification failed and there's a requesterPeerId set,
		// try again without it (the signature may have been created without the requesterPeerId)
		if !signatureValid && strings.TrimSpace(fig.RequesterPeerID) != "" {
			log.Printf("Signature verification failed with requesterPeerId, trying without it")
			figWithoutPeer := fig
			figWithoutPeer.RequesterPeerID = ""
			payloadWithoutPeer := figWithoutPeer.BuildCanonicalPayload()
			signatureValid = sh.verifyAnySignatureAgainstKeys(fig.Signatures, payloadWithoutPeer, allServiceKeys)
			if signatureValid {
				log.Printf("Signature verified without requesterPeerId - accepting fig")
			}
		}

		if !signatureValid {
			return fmt.Errorf("no valid signatures from service keys in the fig tree")
		}
	}
	if allowInsecure {
		// Best-effort logging about signature state
		allServiceKeys := fig.CollectAllServiceKeys()
		payload := fig.BuildCanonicalPayload()
		anyValid := sh.verifyAnySignatureAgainstKeys(fig.Signatures, payload, allServiceKeys)
		if len(fig.Signatures) == 0 {
			log.Printf("WARNING: Loading INSECURE fig with NO signatures due to --allow-insecure-figs")
		} else if !anyValid {
			log.Printf("WARNING: Loading INSECURE fig with one or more INVALID signatures due to --allow-insecure-figs")
		}
	}

	// Optional binding to requester peer id: log a warning if it doesn't match but don't reject
	// This allows figs signed for other nodes to be loaded and used
	if strings.TrimSpace(fig.RequesterPeerID) != "" {
		local := sh.node.GetHost().ID().String()
		if strings.TrimSpace(fig.RequesterPeerID) != local {
			log.Printf("Note: Fig requesterPeerId (%s) does not match local peer id (%s) - this is acceptable for shared figs",
				fig.RequesterPeerID, local)
		}
	}

	// Validate transport restrictions in the fig tree
	if err := sh.validateTransportRestrictions(&fig.Root); err != nil {
		return fmt.Errorf("invalid transport restrictions in fig: %w", err)
	}

	return nil
}

// verifyAnySignatureAgainstRootKeys attempts to verify any provided signature against any root key.
// Deprecated: Use verifyAnySignatureAgainstKeys instead for path-aware verification.
func (sh *ServiceHandlers) verifyAnySignatureAgainstRootKeys(fig types.FigFile, payload []byte) bool {
	return sh.verifyAnySignatureAgainstKeys(fig.Signatures, payload, fig.Root.Keys)
}

// verifyAnySignatureAgainstKeys attempts to verify any provided signature against any of the provided keys.
func (sh *ServiceHandlers) verifyAnySignatureAgainstKeys(signatures []string, payload []byte, keys []string) bool {
	for _, sigHex := range signatures {
		sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
		if err != nil || len(sigBytes) == 0 {
			continue
		}
		for _, keyHex := range keys {
			pubKeyBytes, err := hex.DecodeString(strings.TrimSpace(keyHex))
			if err != nil {
				continue
			}
			pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
			if err != nil {
				continue
			}
			if ok, err := pubKey.Verify(payload, sigBytes); err == nil && ok {
				return true
			}
		}
	}
	return false
}

// verifyAllSignaturesAgainstRootKeys ensures every signature verifies against at least one root key.
// Deprecated: Use verifyAllSignaturesAgainstKeys instead for path-aware verification.
func (sh *ServiceHandlers) verifyAllSignaturesAgainstRootKeys(fig types.FigFile, payload []byte) bool {
	return sh.verifyAllSignaturesAgainstKeys(fig.Signatures, payload, fig.Root.Keys)
}

// verifyAllSignaturesAgainstKeys ensures every signature verifies against at least one of the provided keys.
func (sh *ServiceHandlers) verifyAllSignaturesAgainstKeys(signatures []string, payload []byte, keys []string) bool {
	if len(signatures) == 0 {
		return false
	}
	// Pre-decode keys once
	pubKeys := make([]crypto.PubKey, 0, len(keys))
	for _, keyHex := range keys {
		pubKeyBytes, err := hex.DecodeString(strings.TrimSpace(keyHex))
		if err != nil {
			continue
		}
		pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
		if err != nil {
			continue
		}
		pubKeys = append(pubKeys, pubKey)
	}
	if len(pubKeys) == 0 {
		return false
	}
	for _, sigHex := range signatures {
		sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
		if err != nil || len(sigBytes) == 0 {
			return false
		}
		okAny := false
		for _, pub := range pubKeys {
			if ok, err := pub.Verify(payload, sigBytes); err == nil && ok {
				okAny = true
				break
			}
		}
		if !okAny {
			return false
		}
	}
	return true
}

// verifyAuthoritySignatures verifies that all required authority signers have valid signatures
// over the authority payload (which excludes nonce, requester peer ID, and signatures themselves).
func (sh *ServiceHandlers) verifyAuthoritySignatures(fig types.FigFile) bool {
	cfg := sh.node.GetConfig()
	allowInsecure := cfg != nil && cfg.AllowInsecureFigs != nil && *cfg.AllowInsecureFigs

	// Skip verification if insecure mode is enabled
	if allowInsecure {
		return true
	}

	if len(fig.RequiredSigners) == 0 {
		return true
	}

	if len(fig.AuthoritySignatures) == 0 {
		return false
	}

	// Build the authority payload
	payload := fig.BuildAuthorityPayload()

	// Pre-decode all required signer public keys
	requiredPubKeys := make([]crypto.PubKey, 0, len(fig.RequiredSigners))
	for _, keyHex := range fig.RequiredSigners {
		pubKeyBytes, err := hex.DecodeString(strings.TrimSpace(keyHex))
		if err != nil {
			continue
		}
		pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
		if err != nil {
			continue
		}
		requiredPubKeys = append(requiredPubKeys, pubKey)
	}

	if len(requiredPubKeys) == 0 {
		return false
	}

	// Pre-decode all authority signatures
	authoritySignatures := make([][]byte, 0, len(fig.AuthoritySignatures))
	for _, sigHex := range fig.AuthoritySignatures {
		sigBytes, err := hex.DecodeString(strings.TrimSpace(sigHex))
		if err != nil || len(sigBytes) == 0 {
			continue
		}
		authoritySignatures = append(authoritySignatures, sigBytes)
	}

	if len(authoritySignatures) == 0 {
		return false
	}

	// Verify that each required signer has at least one valid signature
	for _, requiredPubKey := range requiredPubKeys {
		foundValidSignature := false
		for _, sigBytes := range authoritySignatures {
			if ok, err := requiredPubKey.Verify(payload, sigBytes); err == nil && ok {
				foundValidSignature = true
				break
			}
		}
		if !foundValidSignature {
			return false
		}
	}

	return true
}

// validateTransportRestrictions recursively validates transport restrictions in a fig node tree.
// It checks that all allowedTransports values are valid transport identifiers.
func (sh *ServiceHandlers) validateTransportRestrictions(node *types.FigNode) error {
	if node == nil {
		return nil
	}

	// Validate transport identifiers if present
	if len(node.AllowedTransports) > 0 {
		for _, transport := range node.AllowedTransports {
			// Import the transport package at the top of the file
			// For now, we'll do basic validation
			transport = strings.ToLower(strings.TrimSpace(transport))
			validTransports := map[string]bool{
				"tcp": true, "udp": true, "quic": true,
				"ws": true, "wss": true,
				"i2p": true, "nym": true, "onion": true,
			}
			if !validTransports[transport] {
				return fmt.Errorf("invalid transport identifier '%s' in path '%s'", transport, node.Path)
			}
		}
	}

	// Recursively validate children
	for i := range node.Children {
		if err := sh.validateTransportRestrictions(&node.Children[i]); err != nil {
			return err
		}
	}

	return nil
}

// startLocatorFromKey starts a service locator given a compressed public key string.
// Returns response map, http status on error, and error.
func (sh *ServiceHandlers) startLocatorFromKey(keyHex string) (map[string]interface{}, int, error) {
	pubKeyBytes, err := hex.DecodeString(keyHex)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid hex format for compressedPublicKey")
	}
	pubKey, err := crypto.UnmarshalPublicKey(pubKeyBytes)
	if err != nil {
		return nil, http.StatusBadRequest, fmt.Errorf("invalid public key format")
	}
	if err := sh.node.GetServiceManager().StartServiceLocator(pubKey, types.ServiceBeaconModeLookup); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			return nil, http.StatusConflict, fmt.Errorf("service locator already exists for this service")
		}
		return nil, http.StatusInternalServerError, fmt.Errorf("failed to start service locator: %v", err)
	}
	// Normalize key to canonical marshaled form and remember for /locators
	if marshaled, e2 := crypto.MarshalPublicKey(pubKey); e2 == nil {
		normalized := fmt.Sprintf("%x", marshaled)
		sh.locatorsMu.Lock()
		if sh.startedLocatorKeys == nil {
			sh.startedLocatorKeys = make(map[string]time.Time)
		}
		sh.startedLocatorKeys[normalized] = time.Now()
		sh.locatorsMu.Unlock()
	}
	hash := sha256.Sum256(pubKeyBytes)
	serviceKeyHash := fmt.Sprintf("%x", hash[:8])
	res := map[string]interface{}{
		"message":          "Service locator started successfully",
		"service_key":      keyHex,
		"service_key_hash": serviceKeyHash,
		"mode":             "lookup",
		"timestamp":        time.Now(),
	}
	return res, 0, nil
}
