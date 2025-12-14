package libp2phttp

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	gostream "github.com/libp2p/go-libp2p-gostream"
	p2phttp "github.com/libp2p/go-libp2p-http"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/interfaces"
	"banyan/types"
)

// Note: All types and interfaces moved to shared packages

// Handler handles HTTP endpoint logic
type Handler struct {
	host                    host.Host
	ctx                     context.Context
	httpTransport           *http.Transport
	eventBroadcaster        interfaces.EventBroadcaster
	connectionManager       interfaces.PeerManager
	serviceManager          interfaces.ServiceManager
	cryptoManager           interfaces.CryptoManager
	routeTable              *types.RouteTable
	mux                     *http.ServeMux
	addonDisclosureProvider func() []types.AddonDisclosure
	noCrypto                bool
}

// NewHandler creates a new HTTP handler
func NewHandler(host host.Host, ctx context.Context, httpTransport *http.Transport,
	connectionManager interfaces.PeerManager, serviceManager interfaces.ServiceManager, cryptoManager interfaces.CryptoManager, eventBroadcaster interfaces.EventBroadcaster, routeTable *types.RouteTable, noCrypto bool) interfaces.HTTPHandler {
	return &Handler{
		host:              host,
		ctx:               ctx,
		httpTransport:     httpTransport,
		eventBroadcaster:  eventBroadcaster,
		connectionManager: connectionManager,
		serviceManager:    serviceManager,
		cryptoManager:     cryptoManager,
		routeTable:        routeTable,
		noCrypto:          noCrypto,
	}
}

// sendEvent sends an event via the event broadcaster
func (h *Handler) sendEvent(eventType string, data interface{}) {
	if h.eventBroadcaster != nil {
		h.eventBroadcaster.SendEvent(eventType, data)
	}
}

// StartP2PProtocolServer starts the libp2p protocol server using gostream
func (h *Handler) StartP2PProtocolServer() {
	// Create a listener using gostream.Listen
	listener, err := gostream.Listen(h.host, p2phttp.DefaultP2PProtocol)
	if err != nil {
		h.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to create libp2p protocol listener",
			"error":   err.Error(),
		})
		return
	}
	defer listener.Close()

	h.sendEvent(types.EventInfo, map[string]interface{}{
		"message":  "LibP2P protocol server listening",
		"protocol": p2phttp.DefaultP2PProtocol,
	})

	// Create a separate mux for libp2p HTTP
	libp2pMux := http.NewServeMux()
	h.mux = libp2pMux
	libp2pMux.HandleFunc("/hello", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte("Hi!"))
	})

	// Add our custom handlers - SECURITY: only expose safe endpoints to other peers
	//libp2pMux.HandleFunc("/proxy/", h.HandleLibp2pHTTPProxy)
	libp2pMux.HandleFunc("/ping", h.HandlePing)
	libp2pMux.HandleFunc("/greetings", h.HandleGreetings)
	libp2pMux.HandleFunc("/router/", h.HandleRouter)
	libp2pMux.HandleFunc("/discovery/response", h.HandleDiscoveryResponse)
	libp2pMux.HandleFunc("/services/response", h.HandleServiceResponse)
	// NOTE: /connect-to-peer is intentionally NOT exposed here for security
	// Only read-only endpoints should be accessible to other peers
	libp2pMux.HandleFunc("/status", h.HandleStatus)
	libp2pMux.HandleFunc("/connections", h.HandleConnectionsStatus)
	libp2pMux.HandleFunc("/services/figs", h.HandleServiceFigs)
	libp2pMux.HandleFunc("/services/fig/", h.HandleServiceFigByKey)

	server := &http.Server{
		Handler: libp2pMux,
	}
	if err := server.Serve(listener); err != nil {
		h.sendEvent(types.EventError, map[string]interface{}{
			"message": "LibP2P protocol server error",
			"error":   err.Error(),
		})
	}
}

// MountP2P registers a handler on the libp2p HTTP mux.
func (h *Handler) MountP2P(path string, fn func(http.ResponseWriter, *http.Request)) {
	if h.mux != nil {
		h.mux.HandleFunc(path, fn)
	}
}

// SetAddonDisclosureProvider wires a provider for addon disclosures included in greetings
func (h *Handler) SetAddonDisclosureProvider(provider func() []types.AddonDisclosure) {
	h.addonDisclosureProvider = provider
}

// HandleDiscoveryResponse accepts direct discovery lookup responses and connects
func (h *Handler) HandleDiscoveryResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var resp types.LookupResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}

	// Enforce crypto policy: allow plaintext only if crypto disabled
	if !h.noCrypto && resp.Encrypted == false && len(resp.Addresses) > 0 {
		http.Error(w, "unencrypted addresses rejected", http.StatusBadRequest)
		return
	}

	addresses := resp.Addresses
	if resp.Encrypted {
		if len(resp.PublicKey) == 0 || len(resp.EncryptedData) == 0 || len(resp.Nonce) == 0 {
			http.Error(w, "missing encryption fields", http.StatusBadRequest)
			return
		}
		plaintext, err := h.cryptoManager.DecryptWithPublicKey(resp.EncryptedData, resp.Nonce, resp.PublicKey)
		if err != nil {
			http.Error(w, "decrypt failed", http.StatusBadRequest)
			return
		}
		var addrs []string
		if err := json.Unmarshal(plaintext, &addrs); err != nil {
			http.Error(w, "invalid decrypted payload", http.StatusBadRequest)
			return
		}
		addresses = addrs
	}

	if resp.From == "" {
		http.Error(w, "missing responder PeerID", http.StatusBadRequest)
		return
	}
	pid, err := peer.Decode(resp.From)
	if err != nil {
		http.Error(w, "invalid responder PeerID", http.StatusBadRequest)
		return
	}
	if len(addresses) > 0 {
		h.connectionManager.ConnectToRequesterDirectly(pid, addresses)
	} else {
		_ = h.connectionManager.ConnectToPeer(pid)
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

// HandleServiceResponse handles direct service lookup responses sent via libp2p HTTP
func (h *Handler) HandleServiceResponse(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	defer r.Body.Close()
	var resp types.ServiceResponse
	if err := json.NewDecoder(r.Body).Decode(&resp); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	// Delegate processing to service manager which will decrypt and connect by PeerID
	h.serviceManager.ProcessServiceResponse(&resp)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{"status": "ok"})
}

// Note: Data in figs originates from the peer's local fig JSON.

// HandlePing responds to ping requests
func (h *Handler) HandlePing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"peer_id": h.host.ID().String(),
		"method":  r.Method,
		"path":    r.URL.Path,
	})
}

// HandleGreetings handles greeting requests - enhanced to replace service-lookup functionality
func (h *Handler) HandleGreetings(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" && r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	response := types.GreetingResponse{
		PeerID:    h.host.ID().String(),
		Timestamp: time.Now(),
	}

	// Enhanced service information - replaces service-lookup functionality
	serviceInfo := make(map[string]interface{})

	// Include service key if available
	if h.cryptoManager.HasServiceKey() {
		serviceKey := h.cryptoManager.GetServiceKey()
		if serviceKey != nil {
			servicePubKey := serviceKey.GetPublic()
			servicePubKeyBytes, err := crypto.MarshalPublicKey(servicePubKey)
			if err == nil {
				response.ServiceKey = servicePubKeyBytes
				serviceInfo["has_service"] = true
				serviceInfo["service_active"] = true
			}
		}
	} else {
		serviceInfo["has_service"] = false
	}

	// Add HTTP capability information
	serviceInfo["http_capable"] = true
	serviceInfo["libp2p_version"] = "go-libp2p"
	serviceInfo["protocol_version"] = "1.0.0"

	// Add node addresses for peer discovery (replaces lookup/response functionality)
	addrs := h.host.Addrs()
	addrStrings := make([]string, len(addrs))
	for i, addr := range addrs {
		addrStrings[i] = addr.String()
	}
	serviceInfo["addresses"] = addrStrings

	// Include addon disclosures if available
	if h.addonDisclosureProvider != nil {
		discs := h.addonDisclosureProvider()
		if len(discs) > 0 {
			addons := make([]map[string]interface{}, 0, len(discs))
			for _, d := range discs {
				if strings.TrimSpace(d.Name) == "" {
					continue
				}
				item := map[string]interface{}{"name": d.Name}
				if d.Version != "" {
					item["version"] = d.Version
				}
				if d.Info != nil {
					item["info"] = d.Info
				}
				addons = append(addons, item)
			}
			if len(addons) > 0 {
				serviceInfo["addons"] = addons
			}
		}
	}

	// Include service information in response
	response.Data = serviceInfo

	// Sign the response
	responseData := fmt.Sprintf("%s:%v", response.PeerID, response.Timestamp.Unix())
	if signature, err := h.host.Peerstore().PrivKey(h.host.ID()).Sign([]byte(responseData)); err == nil {
		response.Signature = signature
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(response)
}

// HandleServiceFigs returns the public service keys this node serves and a signed fig per beacon.
// Request body may include either {"nonce":"hex"} or {"requesterPeerId":"peerid"}.
// At least one must be included unless insecure figs are allowed by the receiving validator.
func (h *Handler) HandleServiceFigs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		Nonce           string `json:"nonce"`
		RequesterPeerID string `json:"requesterPeerId"`
		Alias           string `json:"alias"`
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

	// Enumerate beacons
	beacons := h.serviceManager.ListServiceBeacons()

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
	}{PeerID: h.host.ID().String(), Time: time.Now()}

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
			for _, tpl := range tpls {
				g.templates = append(g.templates, tpl)
			}
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
			// If requesterPeerId not provided, try to infer from the libp2p connection
			inferredPeer := strings.TrimSpace(req.RequesterPeerID)
			if inferredPeer == "" {
				if p := h.inferPeerIDFromRequest(r); p != "" {
					inferredPeer = p
				}
			}
			if inferredPeer != "" {
				fig.RequesterPeerID = inferredPeer
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

			// If requesterPeerId not provided, try to infer from the libp2p connection
			inferredPeer := strings.TrimSpace(req.RequesterPeerID)
			if inferredPeer == "" {
				if p := h.inferPeerIDFromRequest(r); p != "" {
					inferredPeer = p
				}
			}
			if inferredPeer != "" {
				fig.RequesterPeerID = inferredPeer
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

// HandleServiceFigByKey returns a signed fig file for a specific service key.
// URL format: /services/fig/{compressedPublicKeyHex}
// Request body may include either {"nonce":"hex"} or {"requesterPeerId":"peerid"}.
// This endpoint allows another node to request a signed fig file for a specific service key
// that this node is serving, which can then be used to resolve to an alias.
func (h *Handler) HandleServiceFigByKey(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost && r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
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
	beacons := h.serviceManager.ListServiceBeacons()
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

	// If requesterPeerId not provided, try to infer from the libp2p connection
	inferredPeer := strings.TrimSpace(req.RequesterPeerID)
	if inferredPeer == "" {
		if p := h.inferPeerIDFromRequest(r); p != "" {
			inferredPeer = p
		}
	}
	if inferredPeer != "" {
		fig.RequesterPeerID = inferredPeer
	}
	if strings.TrimSpace(req.Nonce) != "" {
		fig.Nonce = strings.TrimSpace(req.Nonce)
	}

	// Build canonical payload and sign with the service key
	payload := fig.BuildCanonicalPayload()
	sig, err := priv.Sign(payload)
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to sign fig: %v", err), http.StatusInternalServerError)
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
		"peerId":         h.host.ID().String(),
		"serviceKey":     keyHex,
		"serviceKeyHash": keyHash,
		"alias":          alias,
		"fig":            fig,
		"time":           time.Now(),
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(response)
}

// inferPeerIDFromRequest attempts to extract the remote peer ID for this HTTP request.
// With go-libp2p-http, requests are transported over a libp2p stream. We try common hints
// first (explicit header), then fall back to reverse-lookup via connection manager if available.
func (h *Handler) inferPeerIDFromRequest(r *http.Request) string {
	// 1) Custom header hint if the transport set it (not relied upon, best-effort)
	if v := strings.TrimSpace(r.Header.Get("X-Libp2p-PeerID")); v != "" {
		if _, err := peer.Decode(v); err == nil {
			return v
		}
	}
	// 2) Try to parse peer id out of RemoteAddr (gostream may embed a multiaddr)
	remote := strings.TrimSpace(r.RemoteAddr)
	if remote != "" {
		// Split on common separators
		seps := []string{"/", " ", "|", ":"}
		parts := []string{remote}
		for _, sep := range seps {
			var next []string
			for _, p := range parts {
				next = append(next, strings.Split(p, sep)...)
			}
			parts = next
		}
		for _, cand := range parts {
			cand = strings.TrimSpace(cand)
			if cand == "" {
				continue
			}
			if _, err := peer.Decode(cand); err == nil {
				return cand
			}
		}
	}
	return ""
}

// HandleStatus returns the node status
func (h *Handler) HandleStatus(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"peer_id":   h.host.ID().String(),
		"status":    "running",
		"timestamp": time.Now(),
		"addrs":     h.host.Addrs(),
	})
}

// HandleConnectionsStatus returns the connections status
func (h *Handler) HandleConnectionsStatus(w http.ResponseWriter, r *http.Request) {
	connections := h.connectionManager.GetConnectionsCopy()

	connData := make([]map[string]interface{}, 0, len(connections))
	for peerID, conn := range connections {
		connInfo := map[string]interface{}{
			"peer_id":         peerID.String(),
			"alias":           conn.Alias,
			"status":          conn.Status,
			"connection_type": conn.ConnectionType,
			"http_capable":    conn.HTTPCapable,
			"http_test":       conn.HTTPTestResult,
			"connected":       conn.Connected,
			"last_activity":   conn.LastActivity,
		}
		if conn.LastDisconnect != nil {
			connInfo["last_disconnect"] = *conn.LastDisconnect
		}
		connData = append(connData, connInfo)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"connections": connData,
		"count":       len(connData),
		"timestamp":   time.Now(),
	})
}

// HandleConnectToPeer handles connect to peer requests
func (h *Handler) HandleConnectToPeer(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Extract peer ID from URL path
	pathParts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(pathParts) < 2 {
		http.Error(w, "Invalid path", http.StatusBadRequest)
		return
	}

	peerIDStr := pathParts[1] // connect-to-peer/{peerID}

	// Check if it's an alias
	if len(peerIDStr) == 4 {
		// Try to resolve alias
		if peerID, exists := h.connectionManager.GetPeerByAlias(peerIDStr); exists {
			peerIDStr = peerID.String()
		}
	}

	// Try to connect
	peerID, err := peer.Decode(peerIDStr)
	if err != nil {
		http.Error(w, "Invalid peer ID", http.StatusBadRequest)
		return
	}

	if err := h.connectionManager.ConnectToPeer(peerID); err != nil {
		http.Error(w, fmt.Sprintf("Connection failed: %v", err), http.StatusInternalServerError)
		return
	}

	// Send response
	h.serviceManager.SendConnectToPeerResponse(peerIDStr)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "connecting",
		"peer_id": peerIDStr,
	})
}

// HandleLibp2pHTTPProxy handles libp2p HTTP proxy requests
func (h *Handler) HandleLibp2pHTTPProxy(w http.ResponseWriter, r *http.Request) {
	// Extract the target URL from the path
	path := strings.TrimPrefix(r.URL.Path, "/proxy/")
	if path == "" {
		http.Error(w, "Missing target URL", http.StatusBadRequest)
		return
	}

	// Decode the URL
	targetURL, err := url.QueryUnescape(path)
	if err != nil {
		http.Error(w, "Invalid URL encoding", http.StatusBadRequest)
		return
	}

	h.sendEvent(types.EventProxyRequest, map[string]interface{}{
		"target": targetURL,
		"method": r.Method,
		"from":   r.RemoteAddr,
	})

	// Create new request to target
	proxyReq, err := http.NewRequestWithContext(h.ctx, r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, v := range r.Header {
		proxyReq.Header[k] = v
	}

	// Make the request using our transport
	client := &http.Client{Transport: h.httpTransport}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Proxy request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		h.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to copy proxy response",
			"error":   err.Error(),
		})
	}

	h.sendEvent(types.EventProxyRequest, map[string]interface{}{
		"target": targetURL,
		"method": r.Method,
		"status": resp.StatusCode,
	})
}

// HandleRouter handles router requests - forwards requests to configured URLs based on identifier
func (h *Handler) HandleRouter(w http.ResponseWriter, r *http.Request) {
	// Extract the path: /router/{identifier}/{path...}
	path := strings.TrimPrefix(r.URL.Path, "/router/")
	if path == "" {
		http.Error(w, "Missing identifier in router path", http.StatusBadRequest)
		return
	}

	// Split path to get identifier and remaining path
	pathParts := strings.SplitN(path, "/", 2)
	identifier := pathParts[0]

	var incomingPath string
	if len(pathParts) > 1 {
		incomingPath = "/" + pathParts[1]
	} else {
		incomingPath = "/"
	}

	// Look up the URL for this identifier
	baseURL, exists := h.routeTable.GetRoute(identifier)
	if !exists {
		http.Error(w, fmt.Sprintf("No route found for identifier: %s", identifier), http.StatusNotFound)
		return
	}

	// Get routing configuration for this identifier
	routingConfig, hasConfig := h.routeTable.GetRouteConfig(identifier)

	// Apply routing transformation
	var targetPath string
	if hasConfig {
		if routingConfig.KeepFullPath {
			// Keep the full incoming path and add prefix if specified
			if routingConfig.RoutePrefix != "" {
				targetPath = strings.TrimRight(routingConfig.RoutePrefix, "/") + incomingPath
			} else {
				targetPath = incomingPath
			}
		} else {
			// Strip the incoming path and use only the prefix
			if routingConfig.RoutePrefix != "" {
				targetPath = routingConfig.RoutePrefix
			} else {
				targetPath = "/"
			}
		}
	} else {
		// No routing config, use incoming path as-is
		targetPath = incomingPath
	}

	// Construct the target URL
	targetURL := strings.TrimRight(baseURL, "/") + targetPath

	// Add query parameters if present
	if r.URL.RawQuery != "" {
		targetURL += "?" + r.URL.RawQuery
	}

	h.sendEvent(types.EventProxyRequest, map[string]interface{}{
		"target":       targetURL,
		"identifier":   identifier,
		"method":       r.Method,
		"from":         r.RemoteAddr,
		"incomingPath": incomingPath,
		"targetPath":   targetPath,
		"routePrefix":  routingConfig.RoutePrefix,
		"keepFullPath": routingConfig.KeepFullPath,
	})

	// Create new request to target
	proxyReq, err := http.NewRequestWithContext(h.ctx, r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create router request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for k, v := range r.Header {
		proxyReq.Header[k] = v
	}

	// Make the request using our transport
	client := &http.Client{Transport: h.httpTransport}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("Router request failed: %v", err), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Copy response headers
	for k, v := range resp.Header {
		w.Header()[k] = v
	}
	w.WriteHeader(resp.StatusCode)

	// Copy response body
	_, err = io.Copy(w, resp.Body)
	if err != nil {
		h.sendEvent(types.EventError, map[string]interface{}{
			"message": "Failed to copy router response",
			"error":   err.Error(),
		})
	}

	h.sendEvent(types.EventProxyRequest, map[string]interface{}{
		"target":     targetURL,
		"identifier": identifier,
		"method":     r.Method,
		"status":     resp.StatusCode,
	})
}
