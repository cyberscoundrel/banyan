package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/x509"
	"encoding/asn1"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// Config structures matching example-config.json
type Config struct {
	AuthorityKeys []string `json:"authorityKeys,omitempty"`
	Nodes         []Node   `json:"nodes"`
	Figs          []Fig    `json:"figs"`
}

type Node struct {
	Name     string    `json:"name"`
	PK       string    `json:"pk"`
	Services []Service `json:"services"`
}

type Service struct {
	Key          string   `json:"key"`
	Figs         []string `json:"figs"`
	RouteURL     string   `json:"routeUrl,omitempty"`
	RoutePrefix  string   `json:"routePrefix,omitempty"`
	KeepFullPath bool     `json:"keepFullPath,omitempty"`
}

type Fig struct {
	Name            string   `json:"name"`
	Root            FigNode  `json:"root"`
	RequiredSigners []string `json:"requiredSigners,omitempty"`
}

type FigNode struct {
	Path     string    `json:"path"`
	Keys     []string  `json:"keys"`
	Children []FigNode `json:"children,omitempty"`
}

// Output structures for services.json
type ServicesConfig map[string]ServiceEntry

type ServiceEntry struct {
	Directory    string   `json:"directory"`
	PEM          string   `json:"pem,omitempty"`
	PEMs         []string `json:"pems,omitempty"`
	Fig          string   `json:"fig,omitempty"`
	Figs         []string `json:"figs,omitempty"`
	RouteURL     string   `json:"routeUrl,omitempty"`
	RoutePrefix  string   `json:"routePrefix,omitempty"`
	KeepFullPath bool     `json:"keepFullPath,omitempty"`
}

// FigFile represents the structure of a fig file
type FigFile struct {
	ServiceAlias        string    `json:"serviceAlias"`
	Root                FigNode   `json:"root"`
	ExpiresAt           time.Time `json:"expiresAt,omitempty"`
	RequiredSigners     []string  `json:"requiredSigners,omitempty"`
	AuthoritySignatures []string  `json:"authoritySignatures,omitempty"`
}

// Global variable for banyan binary path (set via --banyan-path flag)
var banyanPath string

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Usage: generator <config-file> [output-dir] [--banyan-path=<path>]")
		fmt.Println("Example: generator example-config.json ./output --banyan-path=../../dist/apps/node")
		fmt.Println("")
		fmt.Println("Options:")
		fmt.Println("  --banyan-path  Path to banyan binaries directory (contains win/, linux/, osx/)")
		fmt.Println("                 If not specified, scripts will use a placeholder path")
		os.Exit(1)
	}

	configFile := os.Args[1]
	outputDir := "./topology-output"
	banyanPath = "" // default empty, will use placeholder in scripts

	// Parse remaining arguments
	for i := 2; i < len(os.Args); i++ {
		arg := os.Args[i]
		if strings.HasPrefix(arg, "--banyan-path=") {
			banyanPath = strings.TrimPrefix(arg, "--banyan-path=")
		} else if !strings.HasPrefix(arg, "-") && outputDir == "./topology-output" {
			outputDir = arg
		}
	}

	// Load config
	config, err := loadConfig(configFile)
	if err != nil {
		fmt.Printf("Error loading config: %v\n", err)
		os.Exit(1)
	}

	// Generate topology
	if err := generateTopology(config, outputDir); err != nil {
		fmt.Printf("Error generating topology: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Successfully generated topology in: %s\n", outputDir)
	if banyanPath != "" {
		fmt.Printf("Banyan binary path configured: %s\n", banyanPath)
	} else {
		fmt.Println("Note: No --banyan-path specified. Edit generated scripts to set correct banyan path.")
	}
}

func loadConfig(filename string) (*Config, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var config Config
	if err := json.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	return &config, nil
}

func validateConfig(config *Config) error {
	// Build a map of all defined figs
	definedFigs := make(map[string]bool)
	for _, fig := range config.Figs {
		definedFigs[fig.Name] = true
	}

	// Check that all referenced figs exist
	for _, node := range config.Nodes {
		for _, service := range node.Services {
			for _, figName := range service.Figs {
				if !definedFigs[figName] {
					return fmt.Errorf("node %s service %s references undefined fig %s", node.Name, service.Key, figName)
				}
			}
		}
	}

	return nil
}

func generateTopology(config *Config, outputDir string) error {
	// Validate configuration first
	if err := validateConfig(config); err != nil {
		return fmt.Errorf("configuration validation failed: %w", err)
	}

	// Create output directory
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return fmt.Errorf("failed to create output directory: %w", err)
	}

	// Create keys directory
	keysDir := filepath.Join(outputDir, "keys")
	if err := os.MkdirAll(keysDir, 0755); err != nil {
		return fmt.Errorf("failed to create keys directory: %w", err)
	}

	// Generate key mapping (key name -> compressed public key)
	keyMapping := make(map[string]string)

	// Generate authority keys first
	authorityKeys := make(map[string]crypto.PrivKey)
	if len(config.AuthorityKeys) > 0 {
		authorityKeysDir := filepath.Join(keysDir, "authority")
		if err := os.MkdirAll(authorityKeysDir, 0755); err != nil {
			return fmt.Errorf("failed to create authority keys directory: %w", err)
		}

		for _, keyName := range config.AuthorityKeys {
			privKey, err := generateEd25519Key()
			if err != nil {
				return fmt.Errorf("failed to generate authority key %s: %w", keyName, err)
			}
			authorityKeys[keyName] = privKey

			// Get compressed public key for mapping
			pubKey := privKey.GetPublic()
			pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
			if err != nil {
				return fmt.Errorf("failed to marshal public key for authority %s: %w", keyName, err)
			}
			compressedPubKey := fmt.Sprintf("%x", pubKeyBytes)
			keyMapping[keyName+".pem"] = compressedPubKey

			// Save key to authority keys directory
			keyPath := filepath.Join(authorityKeysDir, keyName+".pem")
			if err := savePrivateKeyToPEM(privKey, keyPath); err != nil {
				return fmt.Errorf("failed to save authority key %s: %w", keyName, err)
			}
			fmt.Printf("Generated authority key: %s\n", keyName)
		}
	}

	// Collect all unique service keys
	uniqueKeys := make(map[string]bool)
	for _, node := range config.Nodes {
		for _, service := range node.Services {
			uniqueKeys[service.Key] = true
		}
	}

	// Generate all service keys and store them
	serviceKeys := make(map[string]crypto.PrivKey)
	for keyName := range uniqueKeys {
		privKey, err := generateEd25519Key()
		if err != nil {
			return fmt.Errorf("failed to generate key %s: %w", keyName, err)
		}
		serviceKeys[keyName] = privKey

		// Get compressed public key for mapping
		pubKey := privKey.GetPublic()
		pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
		if err != nil {
			return fmt.Errorf("failed to marshal public key for %s: %w", keyName, err)
		}
		compressedPubKey := fmt.Sprintf("%x", pubKeyBytes)
		keyMapping[keyName+".pem"] = compressedPubKey

		// Save key to keys directory
		keyPath := filepath.Join(keysDir, keyName+".pem")
		if err := savePrivateKeyToPEM(privKey, keyPath); err != nil {
			return fmt.Errorf("failed to save key %s: %w", keyName, err)
		}
		fmt.Printf("Generated service key: %s\n", keyName)
	}

	// Save key mapping to JSON file
	keyMappingPath := filepath.Join(keysDir, "key-mapping.json")
	if err := saveKeyMapping(keyMapping, keyMappingPath); err != nil {
		return fmt.Errorf("failed to save key mapping: %w", err)
	}

	// Generate node directories
	nodesDir := filepath.Join(outputDir, "nodes")
	if err := os.MkdirAll(nodesDir, 0755); err != nil {
		return fmt.Errorf("failed to create nodes directory: %w", err)
	}

	for _, node := range config.Nodes {
		if err := generateNodeDirectory(node, config, nodesDir, keysDir, serviceKeys, authorityKeys); err != nil {
			return fmt.Errorf("failed to generate node %s: %w", node.Name, err)
		}
		fmt.Printf("Generated node: %s\n", node.Name)
	}

	return nil
}

func generateNodeDirectory(node Node, config *Config, nodesDir, globalKeysDir string, serviceKeys, authorityKeys map[string]crypto.PrivKey) error {
	nodeDir := filepath.Join(nodesDir, node.Name)
	if err := os.MkdirAll(nodeDir, 0755); err != nil {
		return fmt.Errorf("failed to create node directory: %w", err)
	}

	// Generate libp2p identity key if pk is true
	var nodePrivKey crypto.PrivKey
	if strings.ToLower(node.PK) == "true" {
		var err error
		nodePrivKey, err = generateEd25519Key()
		if err != nil {
			return fmt.Errorf("failed to generate node private key: %w", err)
		}

		// Save node private key
		nodeKeyPath := filepath.Join(nodeDir, "node-identity.pem")
		if err := savePrivateKeyToPEM(nodePrivKey, nodeKeyPath); err != nil {
			return fmt.Errorf("failed to save node private key: %w", err)
		}
	}

	// Create services directory
	servicesDir := filepath.Join(nodeDir, "services")
	if err := os.MkdirAll(servicesDir, 0755); err != nil {
		return fmt.Errorf("failed to create services directory: %w", err)
	}

	// Build services.json config
	servicesConfig := make(ServicesConfig)

	// Process each service
	for _, service := range node.Services {
		serviceDir := filepath.Join(servicesDir, service.Key)
		if err := os.MkdirAll(serviceDir, 0755); err != nil {
			return fmt.Errorf("failed to create service directory: %w", err)
		}

		// Copy service key to service directory
		serviceKeyPath := filepath.Join(serviceDir, service.Key+".pem")
		if err := savePrivateKeyToPEM(serviceKeys[service.Key], serviceKeyPath); err != nil {
			return fmt.Errorf("failed to save service key: %w", err)
		}

		// Generate fig files for this service
		figPaths := []string{}
		for _, figName := range service.Figs {
			// Find the fig definition
			var figDef *Fig
			for i := range config.Figs {
				if config.Figs[i].Name == figName {
					figDef = &config.Figs[i]
					break
				}
			}

			if figDef == nil {
				return fmt.Errorf("node %s service %s references fig %s which is not defined in config", node.Name, service.Key, figName)
			}

			// Replace key names with compressed public keys in the fig
			figRoot := replaceKeyNamesWithPublicKeys(figDef.Root, serviceKeys)

			// Replace key names with compressed public keys in requiredSigners
			requiredSigners := make([]string, 0, len(figDef.RequiredSigners))
			for _, keyName := range figDef.RequiredSigners {
				var privKey crypto.PrivKey
				var exists bool

				// Check service keys first, then authority keys
				if privKey, exists = serviceKeys[keyName]; !exists {
					privKey, exists = authorityKeys[keyName]
				}

				if exists {
					pubKey := privKey.GetPublic()
					pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
					if err != nil {
						return fmt.Errorf("failed to marshal public key for required signer %s: %w", keyName, err)
					}
					requiredSigners = append(requiredSigners, fmt.Sprintf("%x", pubKeyBytes))
				} else {
					return fmt.Errorf("required signer %s not found in service keys or authority keys", keyName)
				}
			}

			// Generate fig file with serviceAlias set to the fig name
			figFile := FigFile{
				ServiceAlias:    figName,
				Root:            figRoot,
				ExpiresAt:       time.Now().Add(30 * 24 * time.Hour), // 30 days expiry
				RequiredSigners: requiredSigners,
			}

			// Sign the fig file with authority keys if required signers are present
			if err := signFigWithAuthorities(&figFile, figDef, serviceKeys, authorityKeys); err != nil {
				return fmt.Errorf("failed to sign fig %s: %w", figName, err)
			}

			figPath := filepath.Join(serviceDir, figName+".json")
			if err := saveFigFile(figFile, figPath); err != nil {
				return fmt.Errorf("failed to save fig file: %w", err)
			}
			figPaths = append(figPaths, figName+".json")
		}

		// Add to services config
		entry := ServiceEntry{
			Directory:    service.Key,
			PEM:          service.Key + ".pem",
			RouteURL:     service.RouteURL,
			RoutePrefix:  service.RoutePrefix,
			KeepFullPath: service.KeepFullPath,
		}
		if len(figPaths) == 1 {
			entry.Fig = figPaths[0]
		} else if len(figPaths) > 1 {
			entry.Figs = figPaths
		}
		servicesConfig[service.Key] = entry
	}

	// Save services.json
	servicesConfigPath := filepath.Join(servicesDir, "services.json")
	if err := saveServicesConfig(servicesConfig, servicesConfigPath); err != nil {
		return fmt.Errorf("failed to save services config: %w", err)
	}

	// Generate startup script
	if err := generateStartupScript(node, nodeDir, nodePrivKey != nil); err != nil {
		return fmt.Errorf("failed to generate startup script: %w", err)
	}

	return nil
}

func generateEd25519Key() (crypto.PrivKey, error) {
	// Generate Ed25519 key pair
	_, priv, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate Ed25519 key: %w", err)
	}

	// Convert to libp2p crypto.PrivKey
	libp2pPriv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("failed to generate libp2p key: %w", err)
	}

	// Use the generated key bytes
	_ = priv // We'll use libp2p's key generation which is compatible

	return libp2pPriv, nil
}

// replaceKeyNamesWithPublicKeys recursively replaces key names with compressed public key hex strings
func replaceKeyNamesWithPublicKeys(node FigNode, serviceKeys map[string]crypto.PrivKey) FigNode {
	// Create a new node with replaced keys
	newNode := FigNode{
		Path: node.Path,
		Keys: make([]string, len(node.Keys)),
	}

	// Replace each key name with its compressed public key
	for i, keyName := range node.Keys {
		if privKey, exists := serviceKeys[keyName]; exists {
			pubKey := privKey.GetPublic()
			pubKeyBytes, err := crypto.MarshalPublicKey(pubKey)
			if err != nil {
				// If marshaling fails, keep the original key name
				newNode.Keys[i] = keyName
			} else {
				// Convert to hex string (compressed public key)
				newNode.Keys[i] = fmt.Sprintf("%x", pubKeyBytes)
			}
		} else {
			// Key not found, keep original name
			newNode.Keys[i] = keyName
		}
	}

	// Recursively process children
	if len(node.Children) > 0 {
		newNode.Children = make([]FigNode, len(node.Children))
		for i, child := range node.Children {
			newNode.Children[i] = replaceKeyNamesWithPublicKeys(child, serviceKeys)
		}
	}

	return newNode
}

func savePrivateKeyToPEM(privKey crypto.PrivKey, filename string) error {
	// Convert libp2p key to standard Go crypto key
	stdKey, err := crypto.PrivKeyToStdKey(privKey)
	if err != nil {
		return fmt.Errorf("failed to convert to standard key: %w", err)
	}

	var pkcs8Bytes []byte

	// Handle Ed25519 keys specially - check for pointer type
	switch key := stdKey.(type) {
	case *ed25519.PrivateKey:
		// Ed25519 OID: 1.3.101.112
		oid := asn1.ObjectIdentifier{1, 3, 101, 112}

		// Wrap the private key bytes in an OCTET STRING
		privateKeyBytes, err := asn1.Marshal(key.Seed())
		if err != nil {
			return fmt.Errorf("failed to marshal Ed25519 seed: %w", err)
		}

		// Create PKCS8 structure
		pkcs8 := struct {
			Version int
			Algo    struct {
				Algorithm  asn1.ObjectIdentifier
				Parameters asn1.RawValue `asn1:"optional,omitempty"`
			}
			PrivateKey []byte
		}{
			Version: 0,
			Algo: struct {
				Algorithm  asn1.ObjectIdentifier
				Parameters asn1.RawValue `asn1:"optional,omitempty"`
			}{
				Algorithm: oid,
			},
			PrivateKey: privateKeyBytes,
		}

		pkcs8Bytes, err = asn1.Marshal(pkcs8)
		if err != nil {
			return fmt.Errorf("failed to marshal PKCS8: %w", err)
		}
	case ed25519.PrivateKey:
		// Ed25519 OID: 1.3.101.112
		oid := asn1.ObjectIdentifier{1, 3, 101, 112}

		// Wrap the private key bytes in an OCTET STRING
		privateKeyBytes, err := asn1.Marshal(key.Seed())
		if err != nil {
			return fmt.Errorf("failed to marshal Ed25519 seed: %w", err)
		}

		// Create PKCS8 structure
		pkcs8 := struct {
			Version int
			Algo    struct {
				Algorithm  asn1.ObjectIdentifier
				Parameters asn1.RawValue `asn1:"optional,omitempty"`
			}
			PrivateKey []byte
		}{
			Version: 0,
			Algo: struct {
				Algorithm  asn1.ObjectIdentifier
				Parameters asn1.RawValue `asn1:"optional,omitempty"`
			}{
				Algorithm: oid,
			},
			PrivateKey: privateKeyBytes,
		}

		pkcs8Bytes, err = asn1.Marshal(pkcs8)
		if err != nil {
			return fmt.Errorf("failed to marshal PKCS8: %w", err)
		}
	default:
		// For other key types, use standard marshaling
		pkcs8Bytes, err = x509.MarshalPKCS8PrivateKey(stdKey)
		if err != nil {
			return fmt.Errorf("failed to marshal PKCS8 key: %w", err)
		}
	}

	// Create PEM block
	pemBlock := &pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	}

	// Write to file
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	if err := pem.Encode(file, pemBlock); err != nil {
		return fmt.Errorf("failed to encode PEM: %w", err)
	}

	return nil
}

func saveFigFile(figFile FigFile, filename string) error {
	data, err := json.MarshalIndent(figFile, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal fig file: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write fig file: %w", err)
	}

	return nil
}

func saveServicesConfig(config ServicesConfig, filename string) error {
	data, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal services config: %w", err)
	}

	if err := os.WriteFile(filename, data, 0644); err != nil {
		return fmt.Errorf("failed to write services config: %w", err)
	}

	return nil
}

func saveKeyMapping(mapping map[string]string, path string) error {
	data, err := json.MarshalIndent(mapping, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal key mapping: %w", err)
	}

	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write key mapping: %w", err)
	}

	return nil
}

// buildAuthorityPayload constructs the deterministic payload for authority signatures
// Format: serviceAlias|expiresAt|requiredSigners|rootKeys
func buildAuthorityPayload(figFile FigFile) []byte {
	var b strings.Builder

	// Service alias
	b.WriteString(strings.TrimSpace(figFile.ServiceAlias))

	// Expiry timestamp
	if !figFile.ExpiresAt.IsZero() {
		b.WriteString("|")
		b.WriteString(figFile.ExpiresAt.UTC().Format(time.RFC3339))
	}

	// Required signers (sorted for determinism)
	if len(figFile.RequiredSigners) > 0 {
		b.WriteString("|")
		// Sort required signers for deterministic payload
		sortedSigners := make([]string, len(figFile.RequiredSigners))
		copy(sortedSigners, figFile.RequiredSigners)
		// Simple bubble sort for determinism
		for i := 0; i < len(sortedSigners); i++ {
			for j := i + 1; j < len(sortedSigners); j++ {
				if sortedSigners[i] > sortedSigners[j] {
					sortedSigners[i], sortedSigners[j] = sortedSigners[j], sortedSigners[i]
				}
			}
		}
		b.WriteString(strings.Join(sortedSigners, ","))
	}

	// Root keys (collected recursively and sorted)
	rootKeys := collectAllKeys(figFile.Root)
	if len(rootKeys) > 0 {
		b.WriteString("|")
		// Sort for determinism
		for i := 0; i < len(rootKeys); i++ {
			for j := i + 1; j < len(rootKeys); j++ {
				if rootKeys[i] > rootKeys[j] {
					rootKeys[i], rootKeys[j] = rootKeys[j], rootKeys[i]
				}
			}
		}
		b.WriteString(strings.Join(rootKeys, ","))
	}

	return []byte(b.String())
}

// collectAllKeys recursively collects all keys from a FigNode and its children
func collectAllKeys(node FigNode) []string {
	keys := make([]string, 0)
	keys = append(keys, node.Keys...)

	for _, child := range node.Children {
		keys = append(keys, collectAllKeys(child)...)
	}

	return keys
}

// signFigWithAuthorities signs the fig file with all required authority keys
func signFigWithAuthorities(figFile *FigFile, figDef *Fig, serviceKeys, authorityKeys map[string]crypto.PrivKey) error {
	if len(figDef.RequiredSigners) == 0 {
		return nil // No required signers, nothing to sign
	}

	// Build the authority payload
	payload := buildAuthorityPayload(*figFile)

	// Sign with each required signer
	signatures := make([]string, 0, len(figDef.RequiredSigners))
	for _, keyName := range figDef.RequiredSigners {
		var privKey crypto.PrivKey
		var exists bool

		// Check service keys first, then authority keys
		if privKey, exists = serviceKeys[keyName]; !exists {
			privKey, exists = authorityKeys[keyName]
		}

		if !exists {
			return fmt.Errorf("required signer %s not found", keyName)
		}

		// Sign the payload
		signature, err := privKey.Sign(payload)
		if err != nil {
			return fmt.Errorf("failed to sign with key %s: %w", keyName, err)
		}

		signatures = append(signatures, fmt.Sprintf("%x", signature))
	}

	figFile.AuthoritySignatures = signatures
	return nil
}

func generateStartupScript(node Node, nodeDir string, hasPrivKey bool) error {
	// Determine the OS and generate appropriate script
	// Generate both .sh (Unix) and .ps1 (Windows) scripts

	// Unix shell script
	shScript := generateUnixScript(node, hasPrivKey)
	shPath := filepath.Join(nodeDir, "start-node.sh")
	if err := os.WriteFile(shPath, []byte(shScript), 0755); err != nil {
		return fmt.Errorf("failed to write Unix script: %w", err)
	}

	// Windows PowerShell script
	ps1Script := generateWindowsScript(node, hasPrivKey)
	ps1Path := filepath.Join(nodeDir, "start-node.ps1")
	if err := os.WriteFile(ps1Path, []byte(ps1Script), 0644); err != nil {
		return fmt.Errorf("failed to write Windows script: %w", err)
	}

	// Windows batch script
	batScript := generateBatchScript(node, hasPrivKey)
	batPath := filepath.Join(nodeDir, "start-node.bat")
	if err := os.WriteFile(batPath, []byte(batScript), 0644); err != nil {
		return fmt.Errorf("failed to write batch script: %w", err)
	}

	return nil
}

func generateUnixScript(node Node, hasPrivKey bool) string {
	var sb strings.Builder

	sb.WriteString("#!/bin/bash\n")
	sb.WriteString("# Startup script for node: " + node.Name + "\n")
	sb.WriteString("# Generated by topology-configurator\n\n")

	sb.WriteString("# Get the directory where this script is located\n")
	sb.WriteString("SCRIPT_DIR=\"$( cd \"$( dirname \"${BASH_SOURCE[0]}\" )\" && pwd )\"\n\n")

	sb.WriteString("# Path to the banyan executable\n")
	if banyanPath != "" {
		// Use the configured banyan path (convert to Unix-style path)
		unixPath := strings.ReplaceAll(banyanPath, "\\", "/")
		sb.WriteString("# Detect OS and use appropriate binary\n")
		sb.WriteString("if [[ \"$OSTYPE\" == \"darwin\"* ]]; then\n")
		sb.WriteString("    # macOS - detect architecture\n")
		sb.WriteString("    if [[ $(uname -m) == \"arm64\" ]]; then\n")
		sb.WriteString("        BANYAN_EXEC=\"${SCRIPT_DIR}/" + unixPath + "/osx/banyan-arm64\"\n")
		sb.WriteString("    else\n")
		sb.WriteString("        BANYAN_EXEC=\"${SCRIPT_DIR}/" + unixPath + "/osx/banyan-amd64\"\n")
		sb.WriteString("    fi\n")
		sb.WriteString("else\n")
		sb.WriteString("    # Linux\n")
		sb.WriteString("    BANYAN_EXEC=\"${SCRIPT_DIR}/" + unixPath + "/linux/banyan\"\n")
		sb.WriteString("fi\n\n")
	} else {
		// Placeholder path - user needs to edit
		sb.WriteString("# TODO: Set the correct path to banyan binaries\n")
		sb.WriteString("# Detect OS and use appropriate binary\n")
		sb.WriteString("if [[ \"$OSTYPE\" == \"darwin\"* ]]; then\n")
		sb.WriteString("    # macOS - detect architecture\n")
		sb.WriteString("    if [[ $(uname -m) == \"arm64\" ]]; then\n")
		sb.WriteString("        BANYAN_EXEC=\"/path/to/banyan/osx/banyan-arm64\"\n")
		sb.WriteString("    else\n")
		sb.WriteString("        BANYAN_EXEC=\"/path/to/banyan/osx/banyan-amd64\"\n")
		sb.WriteString("    fi\n")
		sb.WriteString("else\n")
		sb.WriteString("    # Linux\n")
		sb.WriteString("    BANYAN_EXEC=\"/path/to/banyan/linux/banyan\"\n")
		sb.WriteString("fi\n\n")
	}

	sb.WriteString("# Check if banyan executable exists\n")
	sb.WriteString("if [ ! -f \"$BANYAN_EXEC\" ]; then\n")
	sb.WriteString("    echo \"Error: banyan executable not found at $BANYAN_EXEC\"\n")
	sb.WriteString("    echo \"Please build the banyan executable or adjust the path in this script\"\n")
	sb.WriteString("    exit 1\n")
	sb.WriteString("fi\n\n")

	sb.WriteString("# Start the node\n")
	sb.WriteString("echo \"Starting node: " + node.Name + "\"\n")
	sb.WriteString("\"$BANYAN_EXEC\" \\\n")

	if hasPrivKey {
		sb.WriteString("    -privkey=\"${SCRIPT_DIR}/node-identity.pem\" \\\n")
	}

	sb.WriteString("    -services-dir=\"${SCRIPT_DIR}/services\" \\\n")
	sb.WriteString("    -listen=\"/ip4/0.0.0.0/tcp/0\" \\\n")
	sb.WriteString("    \"$@\"\n")

	return sb.String()
}

func generateWindowsScript(node Node, hasPrivKey bool) string {
	var sb strings.Builder

	sb.WriteString("# Startup script for node: " + node.Name + "\n")
	sb.WriteString("# Generated by topology-configurator\n\n")

	sb.WriteString("# Get the directory where this script is located\n")
	sb.WriteString("$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path\n\n")

	sb.WriteString("# Path to the banyan executable\n")
	if banyanPath != "" {
		// Use the configured banyan path (convert to Windows-style path)
		winPath := strings.ReplaceAll(banyanPath, "/", "\\")
		sb.WriteString("$BanyanExec = Join-Path $ScriptDir \"" + winPath + "\\win\\banyan.exe\"\n\n")
	} else {
		// Placeholder path - user needs to edit
		sb.WriteString("# TODO: Set the correct path to banyan binaries\n")
		sb.WriteString("$BanyanExec = \"C:\\path\\to\\banyan\\win\\banyan.exe\"\n\n")
	}

	sb.WriteString("# Check if banyan executable exists\n")
	sb.WriteString("if (-not (Test-Path $BanyanExec)) {\n")
	sb.WriteString("    Write-Host \"Error: banyan executable not found at $BanyanExec\" -ForegroundColor Red\n")
	sb.WriteString("    Write-Host \"Please build the banyan executable or adjust the path in this script\" -ForegroundColor Red\n")
	sb.WriteString("    exit 1\n")
	sb.WriteString("}\n\n")

	sb.WriteString("# Start the node\n")
	sb.WriteString("Write-Host \"Starting node: " + node.Name + "\" -ForegroundColor Green\n")

	sb.WriteString("$arguments = @(\n")
	if hasPrivKey {
		sb.WriteString("    \"-privkey=`\"$ScriptDir\\node-identity.pem`\"\",\n")
	}
	sb.WriteString("    \"-services-dir=`\"$ScriptDir\\services`\"\",\n")
	sb.WriteString("    \"-listen=/ip4/0.0.0.0/tcp/0\"\n")
	sb.WriteString(")\n\n")

	sb.WriteString("# Add any additional arguments passed to this script\n")
	sb.WriteString("$arguments += $args\n\n")

	sb.WriteString("& $BanyanExec $arguments\n")

	return sb.String()
}

func generateBatchScript(node Node, hasPrivKey bool) string {
	var sb strings.Builder

	sb.WriteString("@echo off\n")
	sb.WriteString("REM Startup script for node: " + node.Name + "\n")
	sb.WriteString("REM Generated by topology-configurator\n\n")

	sb.WriteString("REM Get the directory where this script is located\n")
	sb.WriteString("set SCRIPT_DIR=%~dp0\n\n")

	sb.WriteString("REM Path to the banyan executable\n")
	if banyanPath != "" {
		// Use the configured banyan path (convert to Windows-style path)
		winPath := strings.ReplaceAll(banyanPath, "/", "\\")
		sb.WriteString("set BANYAN_EXEC=%SCRIPT_DIR%" + winPath + "\\win\\banyan.exe\n\n")
	} else {
		// Placeholder path - user needs to edit
		sb.WriteString("REM TODO: Set the correct path to banyan binaries\n")
		sb.WriteString("set BANYAN_EXEC=C:\\path\\to\\banyan\\win\\banyan.exe\n\n")
	}

	sb.WriteString("REM Check if banyan executable exists\n")
	sb.WriteString("if not exist \"%BANYAN_EXEC%\" (\n")
	sb.WriteString("    echo Error: banyan executable not found at %BANYAN_EXEC%\n")
	sb.WriteString("    echo Please build the banyan executable or adjust the path in this script\n")
	sb.WriteString("    exit /b 1\n")
	sb.WriteString(")\n\n")

	sb.WriteString("REM Start the node\n")
	sb.WriteString("echo Starting node: " + node.Name + "\n")

	sb.WriteString("\"%BANYAN_EXEC%\" ")
	if hasPrivKey {
		sb.WriteString("-privkey=\"%SCRIPT_DIR%node-identity.pem\" ")
	}
	sb.WriteString("-services-dir=\"%SCRIPT_DIR%services\" ")
	sb.WriteString("-listen=\"/ip4/0.0.0.0/tcp/0\" ")
	sb.WriteString("%*\n")

	return sb.String()
}
