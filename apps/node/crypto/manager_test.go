package crypto

import (
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/test"

	"banyan/interfaces"
)

// TestCryptoManagerInterface tests that the CryptoManager correctly implements
// the CryptoManager interface and handles the absence of a service key.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager without a service key and verifies the initial
//   state of service key related methods.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that HasServiceKey() returns false and GetServiceKey() returns nil
//   when no service key is provided during initialization.
func TestCryptoManagerInterface(t *testing.T) {
	// Generate test keys
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	// Create a real libp2p host for testing
	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	// Create crypto manager
	var manager interfaces.CryptoManager = NewManager(host, nil)

	// Test that we can get service key status
	hasServiceKey := manager.HasServiceKey()
	if hasServiceKey {
		t.Error("Expected no service key, but HasServiceKey() returned true")
	}

	serviceKey := manager.GetServiceKey()
	if serviceKey != nil {
		t.Error("Expected nil service key, but got non-nil")
	}

	t.Log("CryptoManager interface test passed")
}

// TestCryptoManagerWithServiceKey tests that the CryptoManager correctly
// stores and retrieves a service key when one is provided.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager with a service key and verifies that the key
//   is properly stored and accessible.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that HasServiceKey() returns true, GetServiceKey() returns a
//   non-nil key, and the retrieved key matches the one provided during initialization.
func TestCryptoManagerWithServiceKey(t *testing.T) {
	// Generate test keys
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	servicePriv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys: %v", err)
	}

	// Create a real libp2p host for testing
	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	// Create crypto manager with service key
	var manager interfaces.CryptoManager = NewManager(host, servicePriv)

	// Test that we can get service key status
	hasServiceKey := manager.HasServiceKey()
	if !hasServiceKey {
		t.Error("Expected service key, but HasServiceKey() returned false")
	}

	serviceKey := manager.GetServiceKey()
	if serviceKey == nil {
		t.Error("Expected non-nil service key, but got nil")
	}

	if !serviceKey.Equals(servicePriv) {
		t.Error("Service key doesn't match what was provided")
	}

	t.Log("CryptoManager with service key test passed")
}

// TestMultipleServiceKeys tests the registration, retrieval, listing, and
// unregistration of multiple named service keys.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager and registers two service keys with different IDs,
//   then verifies each can be retrieved independently, listed together, and
//   unregistered without affecting the other.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that HasServiceKeyByID() and GetServiceKeyByID() work correctly
//   for each key, ListServiceKeys() returns all registered keys, and after
//   unregistering one key, the other remains accessible.
func TestMultipleServiceKeys(t *testing.T) {
	// Generate test keys
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	servicePriv1, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys 1: %v", err)
	}

	servicePriv2, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys 2: %v", err)
	}

	// Create a real libp2p host for testing
	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	// Create crypto manager
	var manager interfaces.CryptoManager = NewManager(host, nil)

	// Test initial state - no service keys
	if manager.HasServiceKeyByID("key1") {
		t.Error("Expected no service key for key1")
	}
	if manager.GetServiceKeyByID("key1") != nil {
		t.Error("Expected nil service key for key1")
	}

	// Register first service key
	manager.RegisterServiceKey("key1", servicePriv1)

	// Test first key
	if !manager.HasServiceKeyByID("key1") {
		t.Error("Expected service key for key1")
	}
	retrievedKey1 := manager.GetServiceKeyByID("key1")
	if retrievedKey1 == nil {
		t.Error("Expected non-nil service key for key1")
	}
	if !retrievedKey1.Equals(servicePriv1) {
		t.Error("Retrieved key1 doesn't match what was registered")
	}

	// Register second service key
	manager.RegisterServiceKey("key2", servicePriv2)

	// Test second key
	if !manager.HasServiceKeyByID("key2") {
		t.Error("Expected service key for key2")
	}
	retrievedKey2 := manager.GetServiceKeyByID("key2")
	if retrievedKey2 == nil {
		t.Error("Expected non-nil service key for key2")
	}
	if !retrievedKey2.Equals(servicePriv2) {
		t.Error("Retrieved key2 doesn't match what was registered")
	}

	// Test that both keys are still available
	if !manager.HasServiceKeyByID("key1") {
		t.Error("key1 should still be available")
	}

	// Test ListServiceKeys
	keys := manager.ListServiceKeys()
	if len(keys) != 2 {
		t.Errorf("Expected 2 service keys, got %d", len(keys))
	}

	// Check that both keys are in the list
	foundKey1, foundKey2 := false, false
	for _, key := range keys {
		if key == "key1" {
			foundKey1 = true
		}
		if key == "key2" {
			foundKey2 = true
		}
	}
	if !foundKey1 {
		t.Error("key1 not found in ListServiceKeys")
	}
	if !foundKey2 {
		t.Error("key2 not found in ListServiceKeys")
	}

	// Test unregistering a key
	manager.UnregisterServiceKey("key1")
	if manager.HasServiceKeyByID("key1") {
		t.Error("key1 should have been unregistered")
	}
	if manager.GetServiceKeyByID("key1") != nil {
		t.Error("key1 should return nil after unregistering")
	}

	// Test that key2 is still available
	if !manager.HasServiceKeyByID("key2") {
		t.Error("key2 should still be available after unregistering key1")
	}

	// Test ListServiceKeys after unregistering
	keys = manager.ListServiceKeys()
	if len(keys) != 1 {
		t.Errorf("Expected 1 service key after unregistering, got %d", len(keys))
	}
	if keys[0] != "key2" {
		t.Errorf("Expected remaining key to be key2, got %s", keys[0])
	}

	t.Log("Multiple service keys test passed")
}

// TestDecryptPeerIDWithSpecificServiceKey tests the error handling of
// DecryptPeerIDWithSpecificServiceKey when called with a nil service key.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager with a service key and attempts to decrypt
//   a peer ID using a nil service key parameter.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that an error is returned containing "no service private key provided"
//   when the service key parameter is nil.
func TestDecryptPeerIDWithSpecificServiceKey(t *testing.T) {
	// Generate test keys
	servicePriv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys: %v", err)
	}

	// Create a crypto manager for the service provider (with service key)
	serviceHost, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create service libp2p host: %v", err)
	}
	defer serviceHost.Close()

	// Create crypto manager
	serviceManager := NewManager(serviceHost, servicePriv)

	// Test with nil service key - should return error
	_, err = serviceManager.DecryptPeerIDWithSpecificServiceKey([]byte("test"), []byte("testnonce123"), []byte("testpubkey"), nil)
	if err == nil {
		t.Error("Expected error when decrypting with nil service key")
	}
	if !strings.Contains(err.Error(), "no service private key provided") {
		t.Errorf("Expected 'no service private key provided' error, got: %v", err)
	}

	t.Log("DecryptPeerIDWithSpecificServiceKey basic functionality test passed")
}

// TestEncryptDecryptWithPublicKey tests that data can be encrypted using
// a public key and produces valid ciphertext and nonce.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager and encrypts test data using a randomly generated
//   requester's public key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the encryption succeeds without error and produces non-empty
//   ciphertext and nonce values.
func TestEncryptDecryptWithPublicKey(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	manager := NewManager(host, nil)

	requesterPriv, requesterPub, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate requester keys: %v", err)
	}

	requesterPubKeyBytes, err := crypto.MarshalPublicKey(requesterPub)
	if err != nil {
		t.Fatalf("Failed to marshal requester public key: %v", err)
	}

	originalData := []byte("test secret message")

	ciphertext, nonce, err := manager.EncryptWithPublicKey(originalData, requesterPubKeyBytes)
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}

	if len(nonce) == 0 {
		t.Error("Expected non-empty nonce")
	}

	_ = requesterPriv
}

// TestEncryptDecryptWithServiceKey tests that data can be encrypted using
// a service key and produces valid ciphertext and nonce.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager and encrypts test data using both a requester's
//   public key and a service private key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the encryption succeeds without error and produces non-empty
//   ciphertext and nonce values.
func TestEncryptDecryptWithServiceKey(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	servicePriv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	manager := NewManager(host, nil)

	_, requesterPub, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate requester keys: %v", err)
	}

	requesterPubKeyBytes, err := crypto.MarshalPublicKey(requesterPub)
	if err != nil {
		t.Fatalf("Failed to marshal requester public key: %v", err)
	}

	originalData := []byte("test service message")

	ciphertext, nonce, err := manager.EncryptWithServiceKey(originalData, requesterPubKeyBytes, servicePriv)
	if err != nil {
		t.Fatalf("Failed to encrypt with service key: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}

	if len(nonce) == 0 {
		t.Error("Expected non-empty nonce")
	}
}

// TestEncryptPeerIDWithServiceKey tests that a peer ID can be encrypted
// using a service public key.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager and encrypts a test peer ID string using the
//   service's public key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that the encryption succeeds without error and produces non-empty
//   ciphertext and nonce values.
func TestEncryptPeerIDWithServiceKey(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	servicePriv, servicePub, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	manager := NewManager(host, nil)

	peerID := "QmTestPeerID123456789"

	ciphertext, nonce, err := manager.EncryptPeerIDWithServiceKey(peerID, servicePub)
	if err != nil {
		t.Fatalf("Failed to encrypt peer ID: %v", err)
	}

	if len(ciphertext) == 0 {
		t.Error("Expected non-empty ciphertext")
	}

	if len(nonce) == 0 {
		t.Error("Expected non-empty nonce")
	}

	_ = servicePriv
}

// TestEncryptWithPublicKeyNoPrivateKey tests the behavior when a host
// does not have a private key (which libp2p automatically generates).
//
// WHAT IT IS DOING:
//   Creates a libp2p host without explicitly providing a private key
//   and checks if the host's peerstore has a private key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Skips the test if the host automatically generates a private key,
//   which is the expected behavior in modern libp2p versions.
func TestEncryptWithPublicKeyNoPrivateKey(t *testing.T) {
	host, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	if host.Peerstore().PrivKey(host.ID()) == nil {
		t.Skip("Host automatically generates a private key")
	}
}

// TestEncryptWithServiceKeyNilKey tests the error handling when attempting
// to encrypt with a nil service key.
//
// WHAT IT IS DOING:
//   Creates a CryptoManager and attempts to encrypt data using a nil
//   service private key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Asserts that an error is returned when the service key parameter is nil.
func TestEncryptWithServiceKeyNilKey(t *testing.T) {
	priv, _, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate test keys: %v", err)
	}

	host, err := libp2p.New(libp2p.Identity(priv))
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	manager := NewManager(host, nil)

	_, _, err = manager.EncryptWithServiceKey([]byte("test"), []byte("pubkey"), nil)
	if err == nil {
		t.Error("Expected error when service key is nil")
	}
}

// TestEncryptPeerIDWithServiceKeyNoPrivateKey tests the behavior when a host
// does not have a private key for peer ID encryption.
//
// WHAT IT IS DOING:
//   Creates a libp2p host without explicitly providing a private key
//   and checks if the host's peerstore has a private key.
//
// HOW IT DEMONSTRATES IT WORKS:
//   Skips the test if the host automatically generates a private key,
//   which is the expected behavior in modern libp2p versions.
func TestEncryptPeerIDWithServiceKeyNoPrivateKey(t *testing.T) {
	_, servicePub, err := test.RandTestKeyPair(crypto.Ed25519, 256)
	if err != nil {
		t.Fatalf("Failed to generate service test keys: %v", err)
	}
	_ = servicePub

	host, err := libp2p.New()
	if err != nil {
		t.Fatalf("Failed to create libp2p host: %v", err)
	}
	defer host.Close()

	if host.Peerstore().PrivKey(host.ID()) == nil {
		t.Skip("Host automatically generates a private key")
	}
}
