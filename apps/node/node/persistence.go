package node

import (
	"context"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"

	"banyan/attest"
	"banyan/peerstore"
	"banyan/types"
)

// Persistence-tree warm-up and maintenance tuning.
const (
	// warmupPerKeyLimit bounds how many persisted peers per service key are
	// considered on startup, so a large table can't make a node self-DDoS.
	warmupPerKeyLimit = 16
	// dialConcurrency bounds simultaneous warm/keepalive dials.
	dialConcurrency = 8
	// dialTimeout bounds a single warm/keepalive dial.
	dialTimeout = 20 * time.Second
	// addrTTL is how long persisted multiaddrs are seeded into the libp2p
	// peerstore before a dial.
	addrTTL = 30 * time.Minute
	// keepaliveInterval is how often dropped persisted service peers are
	// re-dialed. This is what keeps the mesh from silently decaying (the ~47h
	// DHT collapse we had to restart out of by hand).
	keepaliveInterval = 60 * time.Second
	// evictInterval is how often stale entries are pruned.
	evictInterval = 6 * time.Hour
	// entryTTL drops entries with no successful dial for this long. Bounded so
	// rotated identities (topology regen rotates all keys) drain out instead of
	// accumulating forever.
	entryTTL = 14 * 24 * time.Hour
	// maxEntriesPerKey hard-caps stored observations per service key so the
	// tree can't grow without bound (PEX can otherwise keep adding candidates).
	maxEntriesPerKey = 64

	// pexInterval is how often a peer-exchange round runs.
	pexInterval = 90 * time.Second
	// pexInitialDelay lets the first connections settle before the first round.
	pexInitialDelay = 15 * time.Second
	// pexPeersPerKey bounds how many connected peers are queried per key per
	// round (loop/amplification control).
	pexPeersPerKey = 4
	// pexClientTimeout bounds a single /peerstore pull.
	pexClientTimeout = 15 * time.Second
)

// warmDialPersistedPeers loads the persistence tree and dials known-good peers
// on startup so the node re-forms its service mesh without waiting on DHT
// rendezvous. Non-blocking by contract (run in a goroutine).
func (n *Node) warmDialPersistedPeers() {
	n.dialPersistedPeers("warm-up")
}

// runPeerStoreMaintenance periodically re-dials dropped persisted service peers
// (self-healing keepalive) and evicts stale entries. Runs until the node
// context is cancelled.
func (n *Node) runPeerStoreMaintenance() {
	// Kick an initial PEX round shortly after startup so a cold node that only
	// has a seed connection converges quickly instead of waiting a full tick.
	go func() {
		select {
		case <-n.ctx.Done():
		case <-time.After(pexInitialDelay):
			n.runPEXRound()
		}
	}()

	keepalive := time.NewTicker(keepaliveInterval)
	pex := time.NewTicker(pexInterval)
	evict := time.NewTicker(evictInterval)
	defer keepalive.Stop()
	defer pex.Stop()
	defer evict.Stop()
	for {
		select {
		case <-n.ctx.Done():
			return
		case <-keepalive.C:
			n.dialPersistedPeers("keepalive")
		case <-pex.C:
			n.runPEXRound()
		case <-evict.C:
			if removed, err := n.peerStore.Evict(n.ctx, entryTTL); err != nil {
				fmt.Printf("persistence tree: eviction failed: %v\n", err)
			} else if removed > 0 {
				fmt.Printf("persistence tree: evicted %d stale entries\n", removed)
			}
			if capped, err := n.peerStore.CapPerServiceKey(n.ctx, maxEntriesPerKey); err == nil && capped > 0 {
				fmt.Printf("persistence tree: capped %d excess entries\n", capped)
			}
		}
	}
}

// pexEntryDTO / pexResponseDTO mirror the libp2p /peerstore/<key> response.
type pexEntryDTO struct {
	Subject     string   `json:"subject"`
	Addrs       []string `json:"addrs"`
	ObservedAt  int64    `json:"observedAt"`
	Attestation string   `json:"attestation"`
}

type pexResponseDTO struct {
	ServiceKey string        `json:"serviceKey"`
	Entries    []pexEntryDTO `json:"entries"`
	TTL        int           `json:"ttl"`
}

// runPEXRound pulls attested peer sets from currently-connected peers for every
// service key this node cares about, verifies each attestation, and stores the
// subjects as dial candidates (verify-before-trust: they only become routable
// after this node itself dials and HTTP-verifies them). This is the epidemic
// bootstrap that replaces the manual connect dance: seeded from one connection
// into a fig, a node discovers the rest of that fig's peers.
func (n *Node) runPEXRound() {
	store := n.peerStore
	if store == nil {
		return
	}
	all, err := store.All(n.ctx, 0)
	if err != nil || len(all) == 0 {
		return
	}
	keySet := make(map[string]bool)
	for _, e := range all {
		keySet[e.ServiceKey] = true
	}
	connected := n.host.Network().Peers()
	if len(connected) == 0 {
		return
	}

	self := n.host.ID().String()
	imported := 0
	for keyHex := range keySet {
		queried := 0
		for _, pid := range connected {
			if queried >= pexPeersPerKey {
				break
			}
			entries, perr := n.pullPeerstore(pid, keyHex)
			if perr != nil {
				continue
			}
			queried++
			for _, dto := range entries {
				if dto.Subject == self {
					continue
				}
				raw, derr := base64.StdEncoding.DecodeString(dto.Attestation)
				if derr != nil {
					continue
				}
				att, uerr := attest.Unmarshal(raw)
				if uerr != nil {
					continue
				}
				if ok, _ := att.Verify(); !ok {
					continue
				}
				// The attestation must actually be about this subject and key.
				if att.Claim.Subject != dto.Subject || att.Claim.ServiceKey != keyHex {
					continue
				}
				if err := store.Upsert(n.ctx, peerstore.Entry{
					ServiceKey:  keyHex,
					Observer:    store.Observer(),
					PeerID:      dto.Subject,
					Multiaddrs:  dto.Addrs,
					Source:      peerstore.SourceImported,
					Attestation: raw,
				}); err == nil {
					imported++
				}
			}
		}
	}
	if imported > 0 {
		fmt.Printf("persistence tree: PEX imported %d candidate(s)\n", imported)
		// Dial the freshly learned candidates now; a successful HTTP
		// verification promotes them to routable and enriches what we serve.
		n.dialPersistedPeers("pex")
	}
}

// pullPeerstore fetches a peer's attested observations for one service key over
// the existing libp2p HTTP transport.
func (n *Node) pullPeerstore(pid peer.ID, keyHex string) ([]pexEntryDTO, error) {
	client := &http.Client{Transport: n.httpTransport, Timeout: pexClientTimeout}
	ctx, cancel := context.WithTimeout(n.ctx, pexClientTimeout)
	defer cancel()
	url := fmt.Sprintf("libp2p://%s/peerstore/%s", pid, keyHex)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("peerstore pull status %d", resp.StatusCode)
	}
	var body pexResponseDTO
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	return body.Entries, nil
}

// peerWarm aggregates everything known about one subject peer across the
// service keys it was observed under.
type peerWarm struct {
	addrs map[string]multiaddr.Multiaddr // deduped by string
	keys  map[string][]byte              // service key hex -> raw bytes
}

// dialPersistedPeers seeds addresses from the persistence tree and dials any
// subject peer that is not already connected, re-tagging its service-key
// associations. Successful dials refresh last_success so ranking stays honest.
func (n *Node) dialPersistedPeers(reason string) {
	store := n.peerStore
	if store == nil {
		return
	}
	entries, err := store.All(n.ctx, warmupPerKeyLimit)
	if err != nil {
		fmt.Printf("persistence tree: %s load failed: %v\n", reason, err)
		return
	}
	if len(entries) == 0 {
		return
	}

	byPeer := make(map[peer.ID]*peerWarm)
	var order []peer.ID
	for _, e := range entries {
		pid, err := peer.Decode(e.PeerID)
		if err != nil || pid == n.host.ID() {
			continue
		}
		pw := byPeer[pid]
		if pw == nil {
			pw = &peerWarm{addrs: map[string]multiaddr.Multiaddr{}, keys: map[string][]byte{}}
			byPeer[pid] = pw
			order = append(order, pid)
		}
		for _, as := range e.Multiaddrs {
			if ma, err := multiaddr.NewMultiaddr(as); err == nil {
				pw.addrs[as] = ma
			}
		}
		if kb, err := hex.DecodeString(e.ServiceKey); err == nil && len(kb) > 0 {
			pw.keys[e.ServiceKey] = kb
		}
	}

	sem := make(chan struct{}, dialConcurrency)
	var wg sync.WaitGroup
	var dialed int
	for _, pid := range order {
		// Skip peers we're already connected to (keepalive only wants drops).
		if n.host.Network().Connectedness(pid) == network.Connected {
			continue
		}
		pw := byPeer[pid]
		dialed++
		wg.Add(1)
		sem <- struct{}{}
		go func(pid peer.ID, pw *peerWarm) {
			defer wg.Done()
			defer func() { <-sem }()
			n.dialPersistedPeer(pid, pw)
		}(pid, pw)
	}
	wg.Wait()
	if dialed > 0 {
		fmt.Printf("persistence tree: %s dialed %d dropped/cold peer(s)\n", reason, dialed)
	}
}

// dialPersistedPeer seeds addrs, re-tags service keys, dials, and records
// success into the store on connect.
func (n *Node) dialPersistedPeer(pid peer.ID, pw *peerWarm) {
	addrs := make([]multiaddr.Multiaddr, 0, len(pw.addrs))
	for _, ma := range pw.addrs {
		addrs = append(addrs, ma)
	}
	if len(addrs) > 0 {
		// Seed the libp2p peerstore so the dial can use these directly and skip
		// a DHT lookup.
		n.host.Peerstore().AddAddrs(pid, addrs, addrTTL)
	}
	// Re-tag service-key association as a candidate; a later HTTP verification
	// promotes it to a successful observation.
	for _, kb := range pw.keys {
		n.connectionManager.AddTrackedPeer(pid, types.NewServicePeerOptions(kb, false))
	}

	dctx, cancel := context.WithTimeout(n.ctx, dialTimeout)
	defer cancel()
	if err := n.host.Connect(dctx, peer.AddrInfo{ID: pid, Addrs: addrs}); err != nil {
		return
	}
	now := time.Now()
	for keyHex := range pw.keys {
		_ = n.peerStore.MarkSuccess(n.ctx, keyHex, pid.String(), now)
	}
}
