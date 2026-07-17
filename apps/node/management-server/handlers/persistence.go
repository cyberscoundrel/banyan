package handlers

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"banyan/attest"
	nodePkg "banyan/node"
	"banyan/peerstore"
)

// PersistenceHandlers exposes the durable peer persistence tree over the
// localhost-only management API for operator inspection and seeding. The
// automatic peer-to-peer version rides libp2p (see the /peerstore/<key>
// handler); these endpoints are the human/operator layer.
type PersistenceHandlers struct {
	node *nodePkg.Node
}

func NewPersistenceHandlers(node *nodePkg.Node) *PersistenceHandlers {
	return &PersistenceHandlers{node: node}
}

// persistedEntryDTO is the wire representation of a persisted entry.
type persistedEntryDTO struct {
	ServiceKey  string   `json:"serviceKey"`
	PeerID      string   `json:"peerId"`
	Observer    string   `json:"observer,omitempty"`
	Addrs       []string `json:"addrs"`
	Source      string   `json:"source"`
	LastSeen    int64    `json:"lastSeen"`
	LastSuccess *int64   `json:"lastSuccess,omitempty"`
	Attestation string   `json:"attestation,omitempty"` // base64 of the signed attestation
}

func toDTO(e peerstore.Entry) persistedEntryDTO {
	dto := persistedEntryDTO{
		ServiceKey: e.ServiceKey,
		PeerID:     e.PeerID,
		Observer:   e.Observer,
		Addrs:      e.Multiaddrs,
		Source:     e.Source,
		LastSeen:   e.LastSeen.Unix(),
	}
	if e.LastSuccess != nil {
		ls := e.LastSuccess.Unix()
		dto.LastSuccess = &ls
	}
	if len(e.Attestation) > 0 {
		dto.Attestation = base64.StdEncoding.EncodeToString(e.Attestation)
	}
	return dto
}

// HandleGetPersistedPeers dumps the persisted peer set. Query params:
//
//	?serviceKey=<hex>  - entries for one service key
//	?fig=<alias>       - union of entries for all keys in the loaded fig
//	(none)             - the entire tree
func (ph *PersistenceHandlers) HandleGetPersistedPeers(w http.ResponseWriter, r *http.Request) {
	if !isLocalhost(r) {
		http.Error(w, "Access denied: This endpoint is only accessible from localhost", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed. Use GET", http.StatusMethodNotAllowed)
		return
	}
	store := ph.store(w)
	if store == nil {
		return
	}

	ctx := r.Context()
	var entries []peerstore.Entry
	var err error

	switch {
	case r.URL.Query().Get("serviceKey") != "":
		entries, err = store.ForServiceKey(ctx, r.URL.Query().Get("serviceKey"), 0)
	case r.URL.Query().Get("fig") != "":
		alias := r.URL.Query().Get("fig")
		keys, ok := GetServiceKeysForAlias(alias, "")
		if !ok {
			http.Error(w, fmt.Sprintf("fig alias %q not resolved on this node", alias), http.StatusNotFound)
			return
		}
		seen := map[string]bool{}
		for _, k := range keys {
			part, perr := store.ForServiceKey(ctx, k, 0)
			if perr != nil {
				err = perr
				break
			}
			for _, e := range part {
				id := e.ServiceKey + "|" + e.Observer + "|" + e.PeerID
				if seen[id] {
					continue
				}
				seen[id] = true
				entries = append(entries, e)
			}
		}
	default:
		entries, err = store.All(ctx, 0)
	}
	if err != nil {
		http.Error(w, fmt.Sprintf("persistence tree query failed: %v", err), http.StatusInternalServerError)
		return
	}

	dtos := make([]persistedEntryDTO, 0, len(entries))
	for _, e := range entries {
		dtos = append(dtos, toDTO(e))
	}
	writeJSON(w, map[string]interface{}{
		"entries":   dtos,
		"count":     len(dtos),
		"timestamp": time.Now(),
	})
}

// HandleImportPersistedPeers merges an externally-provided entry set into the
// tree. Per verify-before-trust, imported entries are stored as dial candidates
// only (source=imported, no last_success): they are dialed and must be
// independently HTTP-verified before they become routable. Any attestation
// present must verify, and must match the entry's subject and service key.
func (ph *PersistenceHandlers) HandleImportPersistedPeers(w http.ResponseWriter, r *http.Request) {
	if !isLocalhost(r) {
		http.Error(w, "Access denied: This endpoint is only accessible from localhost", http.StatusForbidden)
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed. Use POST", http.StatusMethodNotAllowed)
		return
	}
	store := ph.store(w)
	if store == nil {
		return
	}

	var req struct {
		Entries []persistedEntryDTO `json:"entries"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid JSON request body", http.StatusBadRequest)
		return
	}

	imported, rejected := 0, 0
	var reasons []string
	for _, dto := range req.Entries {
		if dto.ServiceKey == "" || dto.PeerID == "" {
			rejected++
			reasons = append(reasons, "entry missing serviceKey or peerId")
			continue
		}
		var attestationBytes []byte
		if dto.Attestation != "" {
			raw, err := base64.StdEncoding.DecodeString(dto.Attestation)
			if err != nil {
				rejected++
				reasons = append(reasons, fmt.Sprintf("%s: bad base64 attestation", dto.PeerID))
				continue
			}
			att, err := attest.Unmarshal(raw)
			if err != nil {
				rejected++
				reasons = append(reasons, fmt.Sprintf("%s: unparseable attestation", dto.PeerID))
				continue
			}
			ok, verr := att.Verify()
			if !ok {
				rejected++
				reasons = append(reasons, fmt.Sprintf("%s: attestation failed verification: %v", dto.PeerID, verr))
				continue
			}
			// Attestation must actually be about this subject and key.
			if att.Claim.Subject != dto.PeerID || att.Claim.ServiceKey != dto.ServiceKey {
				rejected++
				reasons = append(reasons, fmt.Sprintf("%s: attestation subject/key mismatch", dto.PeerID))
				continue
			}
			attestationBytes = raw
		}

		// Candidate-only: no LastSuccess. It becomes routable only after this
		// node dials and HTTP-verifies it.
		if err := store.Upsert(r.Context(), peerstore.Entry{
			ServiceKey:  dto.ServiceKey,
			Observer:    store.Observer(),
			PeerID:      dto.PeerID,
			Multiaddrs:  dto.Addrs,
			Source:      peerstore.SourceImported,
			Attestation: attestationBytes,
		}); err != nil {
			rejected++
			reasons = append(reasons, fmt.Sprintf("%s: upsert failed: %v", dto.PeerID, err))
			continue
		}
		imported++
	}

	log.Printf("persistence tree: imported %d entries, rejected %d", imported, rejected)
	writeJSON(w, map[string]interface{}{
		"imported":  imported,
		"rejected":  rejected,
		"reasons":   reasons,
		"note":      "imported entries are dial candidates only until this node HTTP-verifies them",
		"timestamp": time.Now(),
	})
}

func (ph *PersistenceHandlers) store(w http.ResponseWriter) peerstore.PeerStore {
	if ph.node == nil {
		http.Error(w, "LibP2P node not available", http.StatusServiceUnavailable)
		return nil
	}
	store := ph.node.GetPeerStore()
	if store == nil {
		http.Error(w, "Persistence tree is disabled on this node", http.StatusServiceUnavailable)
		return nil
	}
	return store
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(v)
}
