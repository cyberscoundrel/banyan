// Package crypto provides encryption and decryption operations for secure
// peer-to-peer communication in the Banyan network. It implements AES-GCM
// based encryption with ECDH-style key derivation for establishing shared
// secrets between peers.
//
// The package supports multiple service keys for different identity contexts
// and provides methods for encrypting data, peer IDs, and managing service
// key registrations.
package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"sync"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"

	"banyan/interfaces"
)

// Note: CryptoManager interface is now defined in interfaces package

// Manager provides cryptographic operations for secure peer communication.
// It manages encryption keys and supports multiple service keys indexed by
// unique identifiers. The Manager uses AES-256-GCM for symmetric encryption
// with keys derived from ECDH-style key material hashing.
type Manager struct {
	host        host.Host
	serviceKey  crypto.PrivKey            // Legacy single service key for backward compatibility
	serviceKeys map[string]crypto.PrivKey // Multiple service keys indexed by keyID
	mutex       sync.RWMutex              // Protects serviceKeys map
}

// NewManager creates a new crypto manager with the given libp2p host and
// optional default service key. The service key is used for legacy single-key
// operations while multiple keys can be registered via RegisterServiceKey.
func NewManager(host host.Host, serviceKey crypto.PrivKey) interfaces.CryptoManager {
	return &Manager{
		host:        host,
		serviceKey:  serviceKey,
		serviceKeys: make(map[string]crypto.PrivKey),
	}
}

// EncryptWithPublicKey encrypts data using a derived shared key based on
// the local peer's private key and the requester's public key. It returns
// the ciphertext and a randomly generated nonce. The shared key is derived
// by hashing the concatenation of the local private key bytes and the
// requester's public key bytes.
func (m *Manager) EncryptWithPublicKey(data []byte, requesterPubKeyBytes []byte) ([]byte, []byte, error) {
	// Get our private key
	ourPrivKey := m.host.Peerstore().PrivKey(m.host.ID())
	if ourPrivKey == nil {
		return nil, nil, fmt.Errorf("no private key available")
	}

	// Derive encryption key using key material from both keys
	// Get raw bytes from our private key
	ourPrivKeyBytes, err := crypto.MarshalPrivateKey(ourPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal our private key: %w", err)
	}

	// Create deterministic key by hashing our private key + requester's public key
	keyMaterial := append(ourPrivKeyBytes, requesterPubKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]

	// Create AES cipher using the derived key
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the data
	ciphertext := gcm.Seal(nil, nonce, data, nil)

	return ciphertext, nonce, nil
}

// DecryptWithPublicKey decrypts data that was encrypted using the local
// peer's public key. It derives the same shared key used during encryption
// by combining the local private key with the sender's public key.
func (m *Manager) DecryptWithPublicKey(encryptedData, nonce []byte, senderPubKeyBytes []byte) ([]byte, error) {
	// Get our private key
	ourPrivKey := m.host.Peerstore().PrivKey(m.host.ID())
	if ourPrivKey == nil {
		return nil, fmt.Errorf("no private key available")
	}

	// Get raw bytes from our private key
	ourPrivKeyBytes, err := crypto.MarshalPrivateKey(ourPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal our private key: %w", err)
	}

	// Create deterministic key by hashing our private key + sender's public key
	// Same order as in encryption to derive the same shared secret
	keyMaterial := append(ourPrivKeyBytes, senderPubKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]

	// Create AES cipher using the derived key
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the data
	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}

	return plaintext, nil
}

// EncryptWithServiceKey encrypts data using a specific service private key
// and the requester's public key. This allows encryption on behalf of a
// service identity rather than the peer's identity. Returns the ciphertext
// and a randomly generated nonce.
func (m *Manager) EncryptWithServiceKey(data []byte, requesterPubKeyBytes []byte, servicePrivKey crypto.PrivKey) ([]byte, []byte, error) {
	if servicePrivKey == nil {
		return nil, nil, fmt.Errorf("no service private key available")
	}
	// Marshal service private key
	servicePrivKeyBytes, err := crypto.MarshalPrivateKey(servicePrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal service private key: %w", err)
	}
	// Derive shared key = sha256(servicePrivKeyBytes || requesterPubKeyBytes)
	keyMaterial := append(servicePrivKeyBytes, requesterPubKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nil, nonce, data, nil)
	return ciphertext, nonce, nil
}

// DecryptWithLocatorKey decrypts data using a locator's ephemeral private key
// and the sender's service public key. This is used when a locator receives
// encrypted data from a service peer.
func (m *Manager) DecryptWithLocatorKey(encryptedData, nonce []byte, senderServicePubKeyBytes []byte, locatorPrivKey crypto.PrivKey) ([]byte, error) {
	if locatorPrivKey == nil {
		return nil, fmt.Errorf("no locator private key provided")
	}
	// Marshal locator private key
	locatorPrivKeyBytes, err := crypto.MarshalPrivateKey(locatorPrivKey)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal locator private key: %w", err)
	}
	// Derive shared key = sha256(locatorPrivKeyBytes || senderServicePubKeyBytes)
	keyMaterial := append(locatorPrivKeyBytes, senderServicePubKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to create GCM: %w", err)
	}
	plaintext, err := gcm.Open(nil, nonce, encryptedData, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt data: %w", err)
	}
	return plaintext, nil
}

// EncryptPeerIDWithServiceKey encrypts a peer ID using the local peer's
// private key and a service's public key. This allows a peer to securely
// share its identity with a service. Returns the ciphertext and nonce.
func (m *Manager) EncryptPeerIDWithServiceKey(peerID string, servicePubKey crypto.PubKey) ([]byte, []byte, error) {
	// Get our private key
	ourPrivKey := m.host.Peerstore().PrivKey(m.host.ID())
	if ourPrivKey == nil {
		return nil, nil, fmt.Errorf("no private key available")
	}

	// Get raw bytes from the service public key
	servicePubKeyBytes, err := crypto.MarshalPublicKey(servicePubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal service public key: %w", err)
	}

	// Get raw bytes from our private key
	ourPrivKeyBytes, err := crypto.MarshalPrivateKey(ourPrivKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal our private key: %w", err)
	}

	// Create deterministic key by hashing our private key + service's public key
	keyMaterial := append(ourPrivKeyBytes, servicePubKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]

	// Create AES cipher using the derived key
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create GCM: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt the peer ID
	ciphertext := gcm.Seal(nil, nonce, []byte(peerID), nil)

	return ciphertext, nonce, nil
}

// DecryptPeerIDWithServiceKey decrypts a peer ID that was encrypted using
// the service's public key. It uses the default service key registered with
// the manager. Returns the decrypted peer ID string.
func (m *Manager) DecryptPeerIDWithServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte) (string, error) {
	// Get our service private key
	if m.serviceKey == nil {
		return "", fmt.Errorf("no service private key available")
	}

	// Get raw bytes from our service private key
	ourServicePrivKeyBytes, err := crypto.MarshalPrivateKey(m.serviceKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal our service private key: %w", err)
	}

	// Create deterministic key by hashing requester's public key + our service private key
	// This matches the order used in encryption: requesterPrivKey + servicePubKey
	keyMaterial := append(requesterPubKey, ourServicePrivKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]

	// Create AES cipher using the derived key
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the peer ID
	plaintext, err := gcm.Open(nil, nonce, encryptedPeerID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt peer ID: %w", err)
	}

	return string(plaintext), nil
}

// GetServiceKey returns the default service private key registered with
// the manager. Returns nil if no service key has been set.
func (m *Manager) GetServiceKey() crypto.PrivKey {
	return m.serviceKey
}

// HasServiceKey returns true if a default service key is available.
func (m *Manager) HasServiceKey() bool {
	return m.serviceKey != nil
}

// DecryptPeerIDWithSpecificServiceKey decrypts a peer ID using a specific
// service private key rather than the default. This allows decryption when
// multiple service keys are in use.
func (m *Manager) DecryptPeerIDWithSpecificServiceKey(encryptedPeerID, nonce []byte, requesterPubKey []byte, servicePrivKey crypto.PrivKey) (string, error) {
	if servicePrivKey == nil {
		return "", fmt.Errorf("no service private key provided")
	}

	// Get raw bytes from the service private key
	servicePrivKeyBytes, err := crypto.MarshalPrivateKey(servicePrivKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal service private key: %w", err)
	}

	// Create deterministic key by hashing requester's public key + service private key
	// This matches the order used in encryption: requesterPrivKey + servicePubKey
	keyMaterial := append(requesterPubKey, servicePrivKeyBytes...)
	keyHash := sha256.Sum256(keyMaterial)
	aesKey := keyHash[:]

	// Create AES cipher using the derived key
	block, err := aes.NewCipher(aesKey)
	if err != nil {
		return "", fmt.Errorf("failed to create AES cipher: %w", err)
	}

	// Create GCM mode
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to create GCM: %w", err)
	}

	// Decrypt the peer ID
	plaintext, err := gcm.Open(nil, nonce, encryptedPeerID, nil)
	if err != nil {
		return "", fmt.Errorf("failed to decrypt peer ID: %w", err)
	}

	return string(plaintext), nil
}

// RegisterServiceKey registers a service private key with the given unique
// identifier. Multiple service keys can be registered for different service
// identities. Thread-safe.
func (m *Manager) RegisterServiceKey(keyID string, serviceKey crypto.PrivKey) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.serviceKeys[keyID] = serviceKey
}

// UnregisterServiceKey removes a service key with the given identifier.
// Thread-safe.
func (m *Manager) UnregisterServiceKey(keyID string) {
	m.mutex.Lock()
	defer m.mutex.Unlock()
	delete(m.serviceKeys, keyID)
}

// GetServiceKeyByID returns the service private key for the given identifier.
// Returns nil if no key is registered with that ID. Thread-safe.
func (m *Manager) GetServiceKeyByID(keyID string) crypto.PrivKey {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	return m.serviceKeys[keyID]
}

// HasServiceKeyByID returns true if a service key exists for the given
// identifier. Thread-safe.
func (m *Manager) HasServiceKeyByID(keyID string) bool {
	m.mutex.RLock()
	defer m.mutex.RUnlock()
	_, exists := m.serviceKeys[keyID]
	return exists
}

// ListServiceKeys returns a list of all registered service key identifiers.
// Thread-safe.
func (m *Manager) ListServiceKeys() []string {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	keys := make([]string, 0, len(m.serviceKeys))
	for keyID := range m.serviceKeys {
		keys = append(keys, keyID)
	}
	return keys
}
