package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"banyan/ledger"
)

var (
	configFile  = flag.String("config", "", "Path to JSON configuration file")
	listenAddr  = flag.String("listen", ":8080", "Address to listen on")
	dataDir     = flag.String("data-dir", "./data", "Directory for data storage")
	nodeID      = flag.String("node-id", "", "This node's peer ID")
	staticKey   = flag.Bool("static-key", true, "Has chat-static-key (serve webapp)")
	ledgerKey   = flag.Bool("ledger-key", false, "Has chat-ledger-key (handle posts/sync)")
	modKey      = flag.Bool("mod-key", false, "Has chat-mod-key (handle moderation)")
	adminKey    = flag.Bool("admin-key", false, "Has chat-admin-key (handle admin)")
	syncPeers   = flag.String("sync-peers", "", "Comma-separated list of peer URLs for ledger sync")
	syncEnabled = flag.Bool("sync", true, "Enable periodic ledger sync (ledger-key nodes only)")
)

func main() {
	flag.Parse()

	_, cancel := context.WithCancel(context.Background())
	defer cancel()

	cfg, err := loadConfig(*configFile)
	if err != nil {
		log.Printf("Warning: failed to load config file: %v", err)
		cfg = &Config{}
	}
	mergeFlags(cfg)

	svc, err := NewChatService(cfg)
	if err != nil {
		log.Fatalf("Failed to create chat service: %v", err)
	}
	defer svc.Close()

	if *ledgerKey && *syncEnabled && *syncPeers != "" {
		svc.StartSync()
	}

	server := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      svc.Handler(),
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  120 * time.Second,
	}

	go func() {
		log.Printf("Starting chat service on %s", cfg.ListenAddr)
		log.Printf("Keys: static=%v ledger=%v mod=%v admin=%v",
			cfg.HasStaticKey, cfg.HasLedgerKey, cfg.HasModKey, cfg.HasAdminKey)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server error: %v", err)
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	log.Println("Shutting down...")

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("Server shutdown error: %v", err)
	}

	log.Println("Server stopped")
}

type Config struct {
	ListenAddr   string   `json:"listenAddr"`
	DataDir      string   `json:"dataDir"`
	NodeID       string   `json:"nodeId"`
	HasStaticKey bool     `json:"hasStaticKey"`
	HasLedgerKey bool     `json:"hasLedgerKey"`
	HasModKey    bool     `json:"hasModKey"`
	HasAdminKey  bool     `json:"hasAdminKey"`
	SyncPeers    []string `json:"syncPeers"`
	SyncEnabled  bool     `json:"syncEnabled"`
	SyncInterval string   `json:"syncInterval"`
	ForwardPeers []string `json:"forwardPeers"`
}

func loadConfig(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return &cfg, nil
}

func mergeFlags(cfg *Config) {
	if *listenAddr != ":8080" {
		cfg.ListenAddr = *listenAddr
	}
	if cfg.ListenAddr == "" {
		cfg.ListenAddr = *listenAddr
	}

	if *dataDir != "./data" {
		cfg.DataDir = *dataDir
	}
	if cfg.DataDir == "" {
		cfg.DataDir = *dataDir
	}

	if *nodeID != "" {
		cfg.NodeID = *nodeID
	}

	cfg.HasStaticKey = *staticKey
	cfg.HasLedgerKey = *ledgerKey
	cfg.HasModKey = *modKey
	cfg.HasAdminKey = *adminKey

	if *syncPeers != "" {
		cfg.SyncPeers = splitCSV(*syncPeers)
	}

	cfg.SyncEnabled = *syncEnabled
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	var result []string
	start := 0
	for i := 0; i <= len(s); i++ {
		if i == len(s) || s[i] == ',' {
			part := s[start:i]
			if part != "" {
				result = append(result, part)
			}
			start = i + 1
		}
	}
	return result
}

type ChatService struct {
	config    *Config
	ledger    ledger.Ledger
	handler   *ServiceHandler
	wsHub     *WebSocketHub
	auth      *AuthManager
	forwarder *Forwarder
	syncMgr   *ledger.SyncManager
}

func NewChatService(cfg *Config) (*ChatService, error) {
	dbPath := cfg.DataDir + "/ledger.db"
	l, err := ledger.NewLedger(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create ledger: %w", err)
	}

	wsHub := NewWebSocketHub()
	go wsHub.Run()

	auth := NewAuthManager(cfg.NodeID)

	svc := &ChatService{
		config:    cfg,
		ledger:    l,
		wsHub:     wsHub,
		auth:      auth,
		forwarder: NewForwarder(cfg),
	}

	svc.handler = NewServiceHandler(svc)

	return svc, nil
}

func (s *ChatService) Handler() http.Handler {
	return s.handler
}

func (s *ChatService) Close() error {
	if s.syncMgr != nil {
		s.syncMgr.Stop()
	}
	s.wsHub.Stop()
	return s.ledger.Close()
}

func (s *ChatService) StartSync() {
	if !s.config.HasLedgerKey {
		return
	}

	interval := 30 * time.Second
	if s.config.SyncInterval != "" {
		if d, err := time.ParseDuration(s.config.SyncInterval); err == nil {
			interval = d
		}
	}

	s.syncMgr = ledger.NewSyncManager(s.ledger, s.config.NodeID, s.config.SyncPeers, interval)
	s.syncMgr.Start()
}

func (s *ChatService) HasKeyForPath(path string) bool {
	switch {
	case path == "/" || path == "/posts" || path == "/channels" || path == "/events":
		return s.config.HasStaticKey
	case len(path) >= 7 && path[:7] == "/ledger":
		return s.config.HasLedgerKey
	case len(path) >= 4 && path[:4] == "/mod":
		return s.config.HasModKey
	case len(path) >= 6 && path[:6] == "/admin":
		return s.config.HasAdminKey
	default:
		return false
	}
}

func (s *ChatService) BroadcastEvent(eventType string, data interface{}) {
	s.wsHub.Broadcast(Event{Type: eventType, Data: data})
}
