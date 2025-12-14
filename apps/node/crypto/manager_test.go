package crypto

import (
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/test"

	"banyan/interfaces"
)

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
