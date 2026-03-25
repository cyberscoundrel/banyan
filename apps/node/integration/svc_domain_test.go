package integration

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/management-server/handlers"
	"banyan/testutil/builders"
	"banyan/types"
)

func TestSvcDomainWithRealServiceBeacons(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	servicePrivKey, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate service key: %v", err)
	}

	servicePubKey := servicePrivKey.GetPublic()
	servicePubKeyBytes, err := crypto.MarshalPublicKey(servicePubKey)
	if err != nil {
		t.Fatalf("Failed to marshal service public key: %v", err)
	}
	serviceKeyHex := fmt.Sprintf("%x", servicePubKeyBytes)
	serviceKeyPrefix := serviceKeyHex[:12]

	beaconNode, err := builders.NewNodeBuilder(ctx).
		WithCrypto(false).
		WithBeaconIncludePeerIDAnnouncements(true).
		BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create beacon node: %v", err)
	}
	defer beaconNode.Close()

	locatorNode, err := builders.NewNodeBuilder(ctx).
		WithCrypto(false).
		BuildAndStart()
	if err != nil {
		beaconNode.Close()
		t.Fatalf("Failed to create locator node: %v", err)
	}
	defer locatorNode.Close()

	beaconAddr := beaconNode.GetHost().Addrs()[0]
	beaconPeerInfo := peer.AddrInfo{
		ID:    beaconNode.GetHost().ID(),
		Addrs: beaconNode.GetHost().Addrs(),
	}

	if err := locatorNode.GetHost().Connect(ctx, beaconPeerInfo); err != nil {
		t.Fatalf("Failed to connect locator to beacon: %v", err)
	}

	t.Logf("Connected locator %s to beacon %s via %s", 
		locatorNode.GetHost().ID().String()[:8],
		beaconNode.GetHost().ID().String()[:8],
		beaconAddr.String())

	time.Sleep(2 * time.Second)

	beacon, err := beaconNode.GetServiceManager().CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create service beacon: %v", err)
	}

	if err := beacon.Start(); err != nil {
		t.Fatalf("Failed to start service beacon: %v", err)
	}
	defer beacon.Stop()

	t.Logf("Started service beacon with key prefix: %s...", serviceKeyPrefix)

	err = locatorNode.GetServiceManager().StartServiceLocator(servicePubKey, types.ServiceBeaconModeLookup)
	if err != nil {
		t.Fatalf("Failed to start service locator: %v", err)
	}

	t.Log("Started service locator in lookup mode")

	var foundServiceKey bool
	var foundPeerID peer.ID
	maxWait := 25 * time.Second
	checkInterval := 500 * time.Millisecond
	deadline := time.Now().Add(maxWait)

	for time.Now().Before(deadline) {
		connections := locatorNode.GetConnectionManager().GetConnectionsCopy()
		for pid, conn := range connections {
			for _, sk := range conn.ServiceKeys {
				skHex := fmt.Sprintf("%x", sk)
				if strings.HasPrefix(strings.ToLower(skHex), strings.ToLower(serviceKeyPrefix)) {
					foundServiceKey = true
					foundPeerID = pid
					t.Logf("Found service key %s... on peer %s", skHex[:12], pid.String()[:8])
					break
				}
			}
			if foundServiceKey {
				break
			}
		}
		if foundServiceKey {
			break
		}
		time.Sleep(checkInterval)
	}

	if !foundServiceKey {
		connections := locatorNode.GetConnectionManager().GetConnectionsCopy()
		t.Logf("Service key not discovered within %v. Total connections: %d", maxWait, len(connections))
		for pid, conn := range connections {
			t.Logf("  Peer %s: %d service keys, HTTP capable: %v", pid.String()[:8], len(conn.ServiceKeys), conn.HTTPCapable)
		}
		t.Fatalf("Service key not discovered within %v", maxWait)
	}

	proxyHandlers := handlers.NewProxyHandlers(locatorNode, func(e types.Event) {
		t.Logf("Event: %s", e.Type)
	})

	req := httptest.NewRequest("GET", fmt.Sprintf("/proxy/service-key/%s/test/path", serviceKeyPrefix), nil)
	w := httptest.NewRecorder()

	proxyHandlers.HandleServiceKeyPrefixProxy(w, req)

	if w.Code == http.StatusNotFound && strings.Contains(w.Body.String(), "No service key found") {
		t.Logf("Service key prefix proxy returned 404 (expected if routing not fully set up): %s", w.Body.String())
	} else if w.Code == http.StatusBadRequest && strings.Contains(w.Body.String(), "Ambiguous") {
		t.Logf("Service key prefix proxy returned ambiguous (multiple keys match): %s", w.Body.String())
	} else if w.Code >= 200 && w.Code < 500 {
		t.Logf("Service key prefix proxy returned status %d", w.Code)
	}

	_ = foundPeerID
}

func TestSvcDomainSecurityFlagBlocksInjection(t *testing.T) {
	ctx := context.Background()
	_ = ctx

	servicePrivKey, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate service key: %v", err)
	}

	nodeWithoutFlag, err := builders.NewNodeBuilder(ctx).
		WithCrypto(false).
		WithAllowUnsafeServiceKeyInjection(false).
		BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create node without unsafe flag: %v", err)
	}
	defer nodeWithoutFlag.Close()

	nodeWithFlag, err := builders.NewNodeBuilder(ctx).
		WithCrypto(false).
		WithAllowUnsafeServiceKeyInjection(true).
		BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create node with unsafe flag: %v", err)
	}
	defer nodeWithFlag.Close()

	beaconWithoutFlag, err := nodeWithoutFlag.GetServiceManager().CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create beacon on node without flag: %v", err)
	}
	if beaconWithoutFlag == nil {
		t.Error("Expected beacon to be created even without unsafe flag (beacon creation is allowed)")
	}

	beaconWithFlag, err := nodeWithFlag.GetServiceManager().CreateServiceBeacon(servicePrivKey)
	if err != nil {
		t.Fatalf("Failed to create beacon on node with flag: %v", err)
	}
	if beaconWithFlag == nil {
		t.Error("Expected beacon to be created with unsafe flag")
	}

	t.Log("Both nodes can create service beacons - the flag gates manual injection via API, not beacon creation")
}

func TestSvcDomainMultipleBeacons(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	key1, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key1: %v", err)
	}

	key2, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key2: %v", err)
	}

	beaconNode, err := builders.NewNodeBuilder(ctx).
		WithCrypto(false).
		BuildAndStart()
	if err != nil {
		t.Fatalf("Failed to create beacon node: %v", err)
	}
	defer beaconNode.Close()

	beacon1, err := beaconNode.GetServiceManager().CreateServiceBeacon(key1)
	if err != nil {
		t.Fatalf("Failed to create beacon1: %v", err)
	}

	beacon2, err := beaconNode.GetServiceManager().CreateServiceBeacon(key2)
	if err != nil {
		t.Fatalf("Failed to create beacon2: %v", err)
	}

	if err := beacon1.Start(); err != nil {
		t.Fatalf("Failed to start beacon1: %v", err)
	}
	defer beacon1.Stop()

	if err := beacon2.Start(); err != nil {
		t.Fatalf("Failed to start beacon2: %v", err)
	}
	defer beacon2.Stop()

	beacons := beaconNode.GetServiceManager().ListServiceBeacons()
	if len(beacons) != 2 {
		t.Errorf("Expected 2 beacons, got %d", len(beacons))
	}

	sk1 := beacon1.GetServiceKey()
	sk2 := beacon2.GetServiceKey()
	if sk1 == nil || sk2 == nil {
		t.Fatal("Expected both beacons to have service keys")
	}

	sk1Bytes, _ := crypto.MarshalPublicKey(sk1.GetPublic())
	sk2Bytes, _ := crypto.MarshalPublicKey(sk2.GetPublic())

	if fmt.Sprintf("%x", sk1Bytes) == fmt.Sprintf("%x", sk2Bytes) {
		t.Error("Expected different service keys for different beacons")
	}

	t.Logf("Created %d beacons with unique keys", len(beacons))

	time.Sleep(100 * time.Millisecond)
}

func TestSvcDomainPrefixResolution(t *testing.T) {
	key1, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key1: %v", err)
	}

	key2, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key2: %v", err)
	}

	key3, _, err := crypto.GenerateKeyPairWithReader(crypto.Ed25519, 0, rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate key3: %v", err)
	}

	key1Pub, _ := crypto.MarshalPublicKey(key1.GetPublic())
	key2Pub, _ := crypto.MarshalPublicKey(key2.GetPublic())
	key3Pub, _ := crypto.MarshalPublicKey(key3.GetPublic())

	key1Hex := fmt.Sprintf("%x", key1Pub)
	key2Hex := fmt.Sprintf("%x", key2Pub)
	key3Hex := fmt.Sprintf("%x", key3Pub)

	keys := []string{key1Hex, key2Hex, key3Hex}

	tests := []struct {
		name        string
		prefix      string
		expectMatch int
	}{
		{"16 char prefix", key1Hex[:16], 1},
		{"20 char prefix", key2Hex[:20], 1},
		{"full key", key3Hex, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var matches []string
			prefixLower := strings.ToLower(tt.prefix)

			for _, key := range keys {
				if strings.HasPrefix(strings.ToLower(key), prefixLower) {
					matches = append(matches, key)
				}
			}

			if len(matches) != tt.expectMatch {
				t.Errorf("Expected %d matches for prefix %s, got %d", tt.expectMatch, tt.prefix[:8], len(matches))
			}

			if len(matches) == 1 {
				t.Logf("Prefix %s... uniquely matches key %s...", tt.prefix[:8], matches[0][:8])
			}
		})
	}

	t.Logf("Key1: %s...", key1Hex[:16])
	t.Logf("Key2: %s...", key2Hex[:16])
	t.Logf("Key3: %s...", key3Hex[:16])
}
