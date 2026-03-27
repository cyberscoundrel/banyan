package ledger

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type SyncClient struct {
	httpClient *http.Client
	selfID     string
	baseURL    string
}

func NewSyncClient(selfID, baseURL string) *SyncClient {
	return &SyncClient{
		httpClient: &http.Client{Timeout: 30 * time.Second},
		selfID:     selfID,
		baseURL:    baseURL,
	}
}

func (s *SyncClient) FetchEntries(ctx context.Context, since string) ([]*Entry, error) {
	url := s.baseURL + "/ledger/sync"
	if since != "" {
		url = url + "?since=" + since
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch entries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sync request failed with status: %d", resp.StatusCode)
	}

	var entries []*Entry
	if err := json.NewDecoder(resp.Body).Decode(&entries); err != nil {
		return nil, fmt.Errorf("failed to decode entries: %w", err)
	}

	return entries, nil
}

func (s *SyncClient) PushEntries(ctx context.Context, entries []*Entry) error {
	body, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("failed to marshal entries: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.baseURL+"/ledger/sync", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := s.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to push entries: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("push request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

type SyncServer struct {
	ledger Ledger
}

func NewSyncServer(ledger Ledger) *SyncServer {
	return &SyncServer{ledger: ledger}
}

func (s *SyncServer) HandleSyncGet(w http.ResponseWriter, r *http.Request) {
	since := r.URL.Query().Get("since")

	entries, err := s.ledger.GetSince(since)
	if err != nil {
		http.Error(w, fmt.Sprintf("failed to get entries: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(entries); err != nil {
		http.Error(w, fmt.Sprintf("failed to encode entries: %v", err), http.StatusInternalServerError)
		return
	}
}

func (s *SyncServer) HandleSyncPost(w http.ResponseWriter, r *http.Request) {
	var entries []*Entry
	if err := json.NewDecoder(r.Body).Decode(&entries); err != nil {
		http.Error(w, fmt.Sprintf("failed to decode entries: %v", err), http.StatusBadRequest)
		return
	}

	if err := s.ledger.Merge(entries); err != nil {
		http.Error(w, fmt.Sprintf("failed to merge entries: %v", err), http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"merged":  len(entries),
		"message": "entries merged successfully",
	})
}

type SyncManager struct {
	client   *SyncClient
	server   *SyncServer
	ledger   Ledger
	peers    []string
	interval time.Duration
	ctx      context.Context
	cancel   context.CancelFunc
}

func NewSyncManager(ledger Ledger, selfID string, peers []string, interval time.Duration) *SyncManager {
	ctx, cancel := context.WithCancel(context.Background())
	return &SyncManager{
		client:   NewSyncClient(selfID, ""),
		ledger:   ledger,
		peers:    peers,
		interval: interval,
		ctx:      ctx,
		cancel:   cancel,
	}
}

func (m *SyncManager) Start() {
	go m.syncLoop()
}

func (m *SyncManager) Stop() {
	m.cancel()
}

func (m *SyncManager) syncLoop() {
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			m.syncWithPeers()
		}
	}
}

func (m *SyncManager) syncWithPeers() {
	for _, peer := range m.peers {
		if err := m.syncWithPeer(peer); err != nil {
			fmt.Printf("sync with peer %s failed: %v\n", peer, err)
			continue
		}
	}
}

func (m *SyncManager) syncWithPeer(peerURL string) error {
	latestHash, err := m.ledger.GetLatestHash()
	if err != nil {
		return fmt.Errorf("failed to get latest hash: %w", err)
	}

	client := NewSyncClient("", peerURL)
	entries, err := client.FetchEntries(m.ctx, latestHash)
	if err != nil {
		return fmt.Errorf("failed to fetch entries: %w", err)
	}

	if len(entries) == 0 {
		return nil
	}

	if err := m.ledger.Merge(entries); err != nil {
		return fmt.Errorf("failed to merge entries: %w", err)
	}

	fmt.Printf("synced %d entries from %s\n", len(entries), peerURL)
	return nil
}

func (m *SyncManager) AddPeer(peerURL string) {
	m.peers = append(m.peers, peerURL)
}

func (m *SyncManager) RemovePeer(peerURL string) {
	for i, p := range m.peers {
		if p == peerURL {
			m.peers = append(m.peers[:i], m.peers[i+1:]...)
			break
		}
	}
}
