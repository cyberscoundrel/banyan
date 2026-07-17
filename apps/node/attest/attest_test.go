package attest

import (
	"crypto/rand"
	"testing"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

func newKey(t *testing.T) (crypto.PrivKey, peer.ID) {
	t.Helper()
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		t.Fatalf("gen key: %v", err)
	}
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		t.Fatalf("id: %v", err)
	}
	return priv, id
}

func TestSignVerifyRoundTrip(t *testing.T) {
	priv, id := newKey(t)
	att, err := Sign(priv, Claim{
		Subject:    "12D3KooWSubject",
		ServiceKey: "0801122000aa",
		Addrs:      []string{"/ip4/1.2.3.4/tcp/4001", "/ip4/0.0.0.0/tcp/1"},
		ObservedAt: 1714000000,
	})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	if att.Claim.Observer != id.String() {
		t.Fatalf("observer not set from key: %q vs %q", att.Claim.Observer, id)
	}

	data, err := att.Marshal()
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}
	parsed, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}
	ok, err := parsed.Verify()
	if err != nil || !ok {
		t.Fatalf("Verify failed: ok=%v err=%v", ok, err)
	}
}

func TestVerifyRejectsTamperedClaim(t *testing.T) {
	priv, _ := newKey(t)
	att, err := Sign(priv, Claim{Subject: "s", ServiceKey: "aa", ObservedAt: 1})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	att.Claim.ServiceKey = "bb" // tamper after signing
	ok, _ := att.Verify()
	if ok {
		t.Fatalf("Verify accepted a tampered claim")
	}
}

func TestVerifyRejectsObserverSpoof(t *testing.T) {
	priv, _ := newKey(t)
	att, err := Sign(priv, Claim{Subject: "s", ServiceKey: "aa", ObservedAt: 1})
	if err != nil {
		t.Fatalf("Sign: %v", err)
	}
	// Claim to be a different observer than the embedded key.
	att.Claim.Observer = "12D3KooWSomeoneElse"
	ok, _ := att.Verify()
	if ok {
		t.Fatalf("Verify accepted an observer/key mismatch")
	}
}
