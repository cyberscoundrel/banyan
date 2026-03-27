package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"
)

type Forwarder struct {
	config     *Config
	httpClient *http.Client
}

func NewForwarder(cfg *Config) *Forwarder {
	return &Forwarder{
		config: cfg,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (f *Forwarder) Forward(r *http.Request) (*http.Response, error) {
	peers := f.getPeersForPath(r.URL.Path)
	if len(peers) == 0 {
		return nil, fmt.Errorf("no peers available for path: %s", r.URL.Path)
	}

	var lastErr error
	for _, peer := range peers {
		resp, err := f.forwardToPeer(r, peer)
		if err != nil {
			lastErr = err
			continue
		}
		return resp, nil
	}

	return nil, fmt.Errorf("all peers failed: %w", lastErr)
}

func (f *Forwarder) getPeersForPath(path string) []string {
	switch {
	case len(path) >= 7 && path[:7] == "/ledger":
	case len(path) >= 4 && path[:4] == "/mod":
	case len(path) >= 6 && path[:6] == "/admin":
	default:
		return nil
	}

	for _, peer := range f.config.ForwardPeers {
		if peer != "" {
			return []string{peer}
		}
	}

	return f.config.ForwardPeers
}

func (f *Forwarder) forwardToPeer(r *http.Request, peerURL string) (*http.Response, error) {
	targetURL, err := url.Parse(peerURL)
	if err != nil {
		return nil, fmt.Errorf("invalid peer URL: %w", err)
	}
	targetURL.Path = r.URL.Path
	targetURL.RawQuery = r.URL.RawQuery

	var body io.Reader
	if r.Body != nil {
		bodyBytes, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, fmt.Errorf("failed to read request body: %w", err)
		}
		r.Body = io.NopCloser(bytes.NewReader(bodyBytes))
		body = bytes.NewReader(bodyBytes)
	}

	req, err := http.NewRequestWithContext(r.Context(), r.Method, targetURL.String(), body)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for key, values := range r.Header {
		for _, value := range values {
			req.Header.Add(key, value)
		}
	}

	req.Header.Set("X-Forwarded-For", r.RemoteAddr)
	req.Header.Set("X-Forwarded-Host", r.Host)
	req.Header.Set("X-Forwarded-Proto", "http")

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to peer failed: %w", err)
	}

	return resp, nil
}

func (f *Forwarder) ForwardWithRetry(r *http.Request, maxRetries int) (*http.Response, error) {
	var lastErr error
	for i := 0; i < maxRetries; i++ {
		resp, err := f.Forward(r)
		if err != nil {
			lastErr = err
			time.Sleep(time.Duration(i+1) * 100 * time.Millisecond)
			continue
		}
		return resp, nil
	}
	return nil, fmt.Errorf("forward failed after %d retries: %w", maxRetries, lastErr)
}

type PeerInfo struct {
	URL       string `json:"url"`
	KeyType   string `json:"keyType"`
	Available bool   `json:"available"`
}

func (f *Forwarder) CheckPeerHealth(peerURL string) (*PeerInfo, error) {
	healthURL := peerURL + "/admin/status"

	resp, err := f.httpClient.Get(healthURL)
	if err != nil {
		return &PeerInfo{URL: peerURL, Available: false}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &PeerInfo{URL: peerURL, Available: false}, fmt.Errorf("health check failed: status %d", resp.StatusCode)
	}

	var status struct {
		Keys struct {
			Static bool `json:"static"`
			Ledger bool `json:"ledger"`
			Mod    bool `json:"mod"`
			Admin  bool `json:"admin"`
		} `json:"keys"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&status); err != nil {
		return &PeerInfo{URL: peerURL, Available: false}, err
	}

	return &PeerInfo{
		URL:       peerURL,
		Available: true,
	}, nil
}
