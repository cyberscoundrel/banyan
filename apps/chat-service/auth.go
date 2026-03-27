package main

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

type Session struct {
	PeerID    string
	Token     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type Challenge struct {
	Nonce     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

type AuthManager struct {
	nodeID     string
	challenges map[string]*Challenge
	sessions   map[string]*Session
	mu         sync.RWMutex
}

func NewAuthManager(nodeID string) *AuthManager {
	return &AuthManager{
		nodeID:     nodeID,
		challenges: make(map[string]*Challenge),
		sessions:   make(map[string]*Session),
	}
}

func (a *AuthManager) GenerateChallenge() (string, error) {
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	challenge := &Challenge{
		Nonce:     base64.URLEncoding.EncodeToString(nonce),
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(5 * time.Minute),
	}

	a.mu.Lock()
	a.challenges[challenge.Nonce] = challenge
	a.mu.Unlock()

	return challenge.Nonce, nil
}

func (a *AuthManager) VerifyChallenge(nonce string) (*Challenge, error) {
	a.mu.RLock()
	challenge, exists := a.challenges[nonce]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("challenge not found")
	}

	if time.Now().After(challenge.ExpiresAt) {
		a.mu.Lock()
		delete(a.challenges, nonce)
		a.mu.Unlock()
		return nil, fmt.Errorf("challenge expired")
	}

	return challenge, nil
}

func (a *AuthManager) CompleteChallenge(nonce, signature string, pubKey crypto.PubKey) (string, error) {
	challenge, err := a.VerifyChallenge(nonce)
	if err != nil {
		return "", err
	}

	sigBytes, err := base64.URLEncoding.DecodeString(signature)
	if err != nil {
		return "", fmt.Errorf("invalid signature encoding: %w", err)
	}

	valid, err := pubKey.Verify([]byte(challenge.Nonce), sigBytes)
	if err != nil {
		return "", fmt.Errorf("signature verification failed: %w", err)
	}

	if !valid {
		return "", fmt.Errorf("invalid signature")
	}

	peerID, err := peer.IDFromPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to get peer ID: %w", err)
	}

	a.mu.Lock()
	delete(a.challenges, nonce)
	a.mu.Unlock()

	return a.CreateSession(peerID.String())
}

func (a *AuthManager) CreateSession(peerID string) (string, error) {
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", fmt.Errorf("failed to generate token: %w", err)
	}
	token := base64.URLEncoding.EncodeToString(tokenBytes)

	session := &Session{
		PeerID:    peerID,
		Token:     token,
		CreatedAt: time.Now(),
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	a.mu.Lock()
	a.sessions[token] = session
	a.mu.Unlock()

	return token, nil
}

func (a *AuthManager) ValidateSession(token string) (*Session, error) {
	a.mu.RLock()
	session, exists := a.sessions[token]
	a.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("session not found")
	}

	if time.Now().After(session.ExpiresAt) {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return nil, fmt.Errorf("session expired")
	}

	return session, nil
}

func (a *AuthManager) RevokeSession(token string) {
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *AuthManager) Authenticate(r *http.Request) (string, error) {
	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-Session-Token")
	}
	if token == "" {
		cookie, err := r.Cookie("session_token")
		if err == nil {
			token = cookie.Value
		}
	}

	if token == "" {
		return "", fmt.Errorf("no session token provided")
	}

	if len(token) > 7 && token[:7] == "Bearer " {
		token = token[7:]
	}

	session, err := a.ValidateSession(token)
	if err != nil {
		return "", err
	}

	return session.PeerID, nil
}

func (a *AuthManager) Cleanup() {
	a.mu.Lock()
	defer a.mu.Unlock()

	now := time.Now()
	for nonce, challenge := range a.challenges {
		if now.After(challenge.ExpiresAt) {
			delete(a.challenges, nonce)
		}
	}

	for token, session := range a.sessions {
		if now.After(session.ExpiresAt) {
			delete(a.sessions, token)
		}
	}
}

type AuthHandlers struct {
	auth *AuthManager
}

func NewAuthHandlers(auth *AuthManager) *AuthHandlers {
	return &AuthHandlers{auth: auth}
}

func (h *AuthHandlers) HandleChallenge(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	nonce, err := h.auth.GenerateChallenge()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to generate challenge: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"challenge": nonce,
		"expiresIn": "300s",
	})
}

func (h *AuthHandlers) HandleVerify(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	var req struct {
		Challenge string `json:"challenge"`
		Signature string `json:"signature"`
		PublicKey []byte `json:"publicKey"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body: "+err.Error())
		return
	}

	if req.Challenge == "" || req.Signature == "" || len(req.PublicKey) == 0 {
		writeError(w, http.StatusBadRequest, "challenge, signature, and publicKey are required")
		return
	}

	pubKey, err := crypto.UnmarshalPublicKey(req.PublicKey)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid public key: "+err.Error())
		return
	}

	token, err := h.auth.CompleteChallenge(req.Challenge, req.Signature, pubKey)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication failed: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"token":     token,
		"expiresIn": "24h",
	})
}

func (h *AuthHandlers) HandleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	token := r.Header.Get("Authorization")
	if token == "" {
		token = r.Header.Get("X-Session-Token")
	}

	if token != "" {
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}
		h.auth.RevokeSession(token)
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "logged out"})
}

func (h *AuthHandlers) HandleMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	peerID, err := h.auth.Authenticate(r)
	if err != nil {
		writeError(w, http.StatusUnauthorized, "authentication required: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{
		"peerId": peerID,
	})
}
