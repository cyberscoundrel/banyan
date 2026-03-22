package mocks

import (
	"sync"

	"github.com/libp2p/go-libp2p/core/crypto"
)

type MockCryptoManager struct {
	mu              sync.RWMutex
	serviceKeys     map[string]crypto.PrivKey
	defaultKey      crypto.PrivKey
	encryptError    error
	decryptError    error
	encryptedData   []byte
	decryptedData   []byte
	encryptCalls    int
	decryptCalls    int
}

func NewMockCryptoManager() *MockCryptoManager {
	return &MockCryptoManager{
		serviceKeys: make(map[string]crypto.PrivKey),
	}
}

func NewMockCryptoManagerWithKey(key crypto.PrivKey) *MockCryptoManager {
	return &MockCryptoManager{
		serviceKeys: make(map[string]crypto.PrivKey),
		defaultKey:  key,
	}
}

func (m *MockCryptoManager) EncryptWithPublicKey(data []byte, requesterPubKeyBytes []byte) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptCalls++
	if m.encryptError != nil {
		return nil, nil, m.encryptError
	}
	nonce := make([]byte, 12)
	result := m.encryptedData
	if result == nil {
		result = data
	}
	return result, nonce, nil
}

func (m *MockCryptoManager) DecryptWithPublicKey(encryptedData, nonce []byte, senderPubKeyBytes []byte) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decryptCalls++
	if m.decryptError != nil {
		return nil, m.decryptError
	}
	result := m.decryptedData
	if result == nil {
		result = encryptedData
	}
	return result, nil
}

func (m *MockCryptoManager) EncryptWithServiceKey(data []byte, requesterPubKeyBytes []byte, servicePrivKey crypto.PrivKey) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptCalls++
	if m.encryptError != nil {
		return nil, nil, m.encryptError
	}
	nonce := make([]byte, 12)
	result := m.encryptedData
	if result == nil {
		result = data
	}
	return result, nonce, nil
}

func (m *MockCryptoManager) DecryptWithLocatorKey(encryptedData, nonce []byte, senderServicePubKeyBytes []byte, locatorPrivKey crypto.PrivKey) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decryptCalls++
	if m.decryptError != nil {
		return nil, m.decryptError
	}
	result := m.decryptedData
	if result == nil {
		result = encryptedData
	}
	return result, nil
}

func (m *MockCryptoManager) EncryptPeerIDWithServiceKey(peerID string, servicePubKey crypto.PubKey) ([]byte, []byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptCalls++
	if m.encryptError != nil {
		return nil, nil, m.encryptError
	}
	nonce := make([]byte, 12)
	result := m.encryptedData
	if result == nil {
		result = []byte(peerID)
	}
	return result, nonce, nil
}

func (m *MockCryptoManager) DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte, servicePrivKey crypto.PrivKey) (string, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decryptCalls++
	if m.decryptError != nil {
		return "", m.decryptError
	}
	if m.decryptedData != nil {
		return string(m.decryptedData), nil
	}
	return string(encryptedPeerID), nil
}

func (m *MockCryptoManager) RegisterServiceKey(keyID string, serviceKey crypto.PrivKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.serviceKeys[keyID] = serviceKey
}

func (m *MockCryptoManager) UnregisterServiceKey(keyID string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.serviceKeys, keyID)
}

func (m *MockCryptoManager) GetServiceKeyByID(keyID string) crypto.PrivKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.serviceKeys[keyID]
}

func (m *MockCryptoManager) HasServiceKeyByID(keyID string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, exists := m.serviceKeys[keyID]
	return exists
}

func (m *MockCryptoManager) ListServiceKeys() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	keys := make([]string, 0, len(m.serviceKeys))
	for k := range m.serviceKeys {
		keys = append(keys, k)
	}
	return keys
}

func (m *MockCryptoManager) GetServiceKey() crypto.PrivKey {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultKey
}

func (m *MockCryptoManager) HasServiceKey() bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.defaultKey != nil
}

func (m *MockCryptoManager) DecryptPeerIDWithServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte) (string, error) {
	return m.DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce, requesterPubKey, m.defaultKey)
}

func (m *MockCryptoManager) SetEncryptError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptError = err
}

func (m *MockCryptoManager) SetDecryptError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decryptError = err
}

func (m *MockCryptoManager) SetEncryptedData(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.encryptedData = data
}

func (m *MockCryptoManager) SetDecryptedData(data []byte) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.decryptedData = data
}

func (m *MockCryptoManager) GetEncryptCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.encryptCalls
}

func (m *MockCryptoManager) GetDecryptCalls() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.decryptCalls
}

func (m *MockCryptoManager) SetDefaultKey(key crypto.PrivKey) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.defaultKey = key
}
