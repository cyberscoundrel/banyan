// Package attest provides signed peer-observation attestations for the
// persistence tree. An attestation binds a claim ("observer O saw subject S
// serving service key K at these addrs at time T") to O's libp2p identity via
// an Ed25519 signature. It lets a persisted or shared entry be independently
// verified: the importer checks the signature and that the embedded public key
// really is the claimed observer's peer ID.
//
// The peerstore package stays free of libp2p so it can be reused/ported; all
// crypto lives here.
package attest

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Claim is the signed statement. Field order is fixed and addrs are sorted so
// the JSON encoding is deterministic across nodes (stable signing payload).
type Claim struct {
	Observer   string   `json:"observer"`   // observing peer ID
	Subject    string   `json:"subject"`    // observed peer ID
	ServiceKey string   `json:"serviceKey"` // service key hex
	Addrs      []string `json:"addrs"`      // subject dial multiaddrs
	ObservedAt int64    `json:"observedAt"` // unix seconds
}

// Attestation is a Claim plus the observer's public key and signature over the
// canonical claim payload.
type Attestation struct {
	Claim  Claim  `json:"claim"`
	PubKey []byte `json:"pubKey"` // marshalled observer public key
	Sig    []byte `json:"sig"`
}

// canonicalPayload returns the deterministic bytes that are signed/verified.
func (c Claim) canonicalPayload() ([]byte, error) {
	cp := c
	cp.Addrs = append([]string(nil), c.Addrs...)
	sort.Strings(cp.Addrs)
	return json.Marshal(cp)
}

// Sign produces an attestation for claim signed by priv. It fills in
// Claim.Observer from priv's derived peer ID (any prior value is overwritten to
// keep the claim self-consistent).
func Sign(priv crypto.PrivKey, claim Claim) (*Attestation, error) {
	id, err := peer.IDFromPrivateKey(priv)
	if err != nil {
		return nil, fmt.Errorf("attest: derive id: %w", err)
	}
	claim.Observer = id.String()
	payload, err := claim.canonicalPayload()
	if err != nil {
		return nil, fmt.Errorf("attest: payload: %w", err)
	}
	sig, err := priv.Sign(payload)
	if err != nil {
		return nil, fmt.Errorf("attest: sign: %w", err)
	}
	pubBytes, err := crypto.MarshalPublicKey(priv.GetPublic())
	if err != nil {
		return nil, fmt.Errorf("attest: marshal pubkey: %w", err)
	}
	return &Attestation{Claim: claim, PubKey: pubBytes, Sig: sig}, nil
}

// Marshal serializes an attestation to bytes for storage/transport.
func (a *Attestation) Marshal() ([]byte, error) { return json.Marshal(a) }

// Unmarshal parses an attestation from bytes.
func Unmarshal(data []byte) (*Attestation, error) {
	var a Attestation
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("attest: unmarshal: %w", err)
	}
	return &a, nil
}

// Verify checks that the signature is valid and that the embedded public key
// really belongs to the claimed observer (pubkey -> peer ID == Claim.Observer).
// It does NOT vouch for the truth of the claim (whether the subject actually
// serves the key) — that requires the importer to dial and HTTP-verify.
func (a *Attestation) Verify() (bool, error) {
	if a == nil {
		return false, fmt.Errorf("attest: nil attestation")
	}
	pub, err := crypto.UnmarshalPublicKey(a.PubKey)
	if err != nil {
		return false, fmt.Errorf("attest: unmarshal pubkey: %w", err)
	}
	// The public key must correspond to the claimed observer peer ID.
	id, err := peer.IDFromPublicKey(pub)
	if err != nil {
		return false, fmt.Errorf("attest: id from pubkey: %w", err)
	}
	if id.String() != a.Claim.Observer {
		return false, fmt.Errorf("attest: observer mismatch: key=%s claim=%s", id, a.Claim.Observer)
	}
	payload, err := a.Claim.canonicalPayload()
	if err != nil {
		return false, fmt.Errorf("attest: payload: %w", err)
	}
	ok, err := pub.Verify(payload, a.Sig)
	if err != nil {
		return false, fmt.Errorf("attest: verify: %w", err)
	}
	return ok, nil
}
