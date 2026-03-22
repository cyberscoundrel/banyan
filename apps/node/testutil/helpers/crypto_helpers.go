package helpers

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"

	"banyan/types"
)

func GenerateTestKeyPair() (crypto.PrivKey, crypto.PubKey, error) {
	return crypto.GenerateEd25519Key(rand.Reader)
}

func GenerateTestKeyPairPEM() ([]byte, crypto.PrivKey, error) {
	privKey, pubKey, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to generate key pair: %w", err)
	}

	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal private key: %w", err)
	}

	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to marshal public key: %w", err)
	}

	pemData := fmt.Sprintf("-----BEGIN PRIVATE KEY-----\n%x\n-----END PRIVATE KEY-----\n-----BEGIN PUBLIC KEY-----\n%x\n-----END PUBLIC KEY-----\n",
		privKeyBytes, pubKeyBytes)

	return []byte(pemData), privKey, nil
}

func CreateTestFigFile(alias string, keys []string) *types.FigFile {
	return &types.FigFile{
		ServiceAlias: alias,
		Root: types.FigNode{
			Path: "/",
			Keys: keys,
		},
	}
}

func CreateTestFigFileWithChildren(alias string, rootKeys []string, children []types.FigNode) *types.FigFile {
	return &types.FigFile{
		ServiceAlias: alias,
		Root: types.FigNode{
			Path:     "/",
			Keys:     rootKeys,
			Children: children,
		},
	}
}

func CreateHierarchicalFigFile() *types.FigFile {
	return &types.FigFile{
		ServiceAlias: "test-hierarchical",
		Root: types.FigNode{
			Path: "/",
			Keys: []string{"root-key"},
			Children: []types.FigNode{
				{
					Path: "/api",
					Keys: []string{"api-key"},
					Children: []types.FigNode{
						{
							Path: "/api/v1",
							Keys: []string{"api-v1-key"},
						},
						{
							Path: "/api/v2",
							Keys: []string{"api-v2-key"},
						},
					},
				},
				{
					Path: "/admin",
					Keys: []string{"admin-key"},
				},
			},
		},
	}
}

func CreateTestFigFileWithExpiration(alias string, keys []string, expiresAt string) *types.FigFile {
	fig := CreateTestFigFile(alias, keys)
	t, err := time.Parse(time.RFC3339, expiresAt)
	if err == nil {
		fig.ExpiresAt = t
	}
	return fig
}

func CreateTestFigFileWithSignatures(alias string, keys []string, nonce string, requesterPeerID string) *types.FigFile {
	fig := CreateTestFigFile(alias, keys)
	fig.Nonce = nonce
	fig.RequesterPeerID = requesterPeerID
	return fig
}

func GenerateTestPeerID() (peer.ID, crypto.PrivKey, error) {
	privKey, _, err := GenerateTestKeyPair()
	if err != nil {
		return "", nil, err
	}

	peerID, err := peer.IDFromPrivateKey(privKey)
	if err != nil {
		return "", nil, fmt.Errorf("failed to generate peer ID: %w", err)
	}

	return peerID, privKey, nil
}

func WriteTestKeyFile(dir string, name string, privKey crypto.PrivKey) (string, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", fmt.Errorf("failed to create directory: %w", err)
	}

	privKeyBytes, err := crypto.MarshalPrivateKey(privKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal private key: %w", err)
	}

	pubKey := privKey.GetPublic()
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}

	pemData := fmt.Sprintf("-----BEGIN PRIVATE KEY-----\n%x\n-----END PRIVATE KEY-----\n-----BEGIN PUBLIC KEY-----\n%x\n-----END PUBLIC KEY-----\n",
		privKeyBytes, pubKeyBytes)

	filePath := filepath.Join(dir, name)
	if err := os.WriteFile(filePath, []byte(pemData), 0600); err != nil {
		return "", fmt.Errorf("failed to write key file: %w", err)
	}

	return filePath, nil
}

func LoadTestKeyFile(path string) (crypto.PrivKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read key file: %w", err)
	}

	var privKeyHex string
	lines := string(data)
	for i := 0; i < len(lines); i++ {
		if len(lines) > i+16 && lines[i:i+16] == "-----BEGIN PRIV" {
			i += 16
			for j := i; j < len(lines); j++ {
				if len(lines) > j+14 && lines[j:j+14] == "-----END PRIV" {
					privKeyHex = lines[i:j]
					break
				}
			}
			break
		}
	}

	if privKeyHex == "" {
		return nil, fmt.Errorf("no private key found in file")
	}

	var keyBytes []byte
	if _, err := fmt.Sscanf(privKeyHex, "%x", &keyBytes); err != nil {
		return nil, fmt.Errorf("failed to parse key hex: %w", err)
	}

	return crypto.UnmarshalPrivateKey(keyBytes)
}

func PublicKeyToHex(pubKey crypto.PubKey) (string, error) {
	pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
	if err != nil {
		return "", fmt.Errorf("failed to marshal public key: %w", err)
	}
	return fmt.Sprintf("%x", pubKeyBytes), nil
}

func HexToPublicKey(hex string) (crypto.PubKey, error) {
	var keyBytes []byte
	if _, err := fmt.Sscanf(hex, "%x", &keyBytes); err != nil {
		return nil, fmt.Errorf("failed to parse hex: %w", err)
	}
	return crypto.UnmarshalPublicKey(keyBytes)
}
