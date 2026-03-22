// Package transport provides utilities for filtering and managing multiaddress
// transport protocols in peer-to-peer networking contexts.
//
// This package enables applications to restrict peer connections to specific
// transport types such as TCP, QUIC, WebSocket, I2P, Nym mixnet, or Tor.
// It parses libp2p multiaddresses to extract transport identifiers and provides
// filtering functions to select addresses based on allowed transport types.
//
// Supported transports include:
//   - tcp: Standard TCP connections
//   - udp: UDP datagrams
//   - quic: QUIC protocol (HTTP/3)
//   - ws: Unencrypted WebSocket
//   - wss: Secure WebSocket (TLS)
//   - i2p: I2P anonymous network
//   - nym: Nym mixnet
//   - onion: Tor hidden services
//
// Example usage:
//
//	allowed := []string{"quic", "wss"}
//	filtered := FilterMultiaddrsByTransport(peerAddrs, allowed)
package transport

import (
	"fmt"
	"strings"

	"github.com/multiformats/go-multiaddr"
)

// Known transport protocol codes and their string identifiers
const (
	TransportTCP  = "tcp"
	TransportUDP  = "udp"
	TransportQUIC = "quic"
	TransportWS   = "ws"
	TransportWSS  = "wss"
	TransportI2P  = "i2p"
	TransportNym  = "nym"
	TransportTor  = "onion"
	TransportP2P  = "p2p"
)

// ValidTransports is a set of recognized transport identifiers
var ValidTransports = map[string]bool{
	TransportTCP:  true,
	TransportUDP:  true,
	TransportQUIC: true,
	TransportWS:   true,
	TransportWSS:  true,
	TransportI2P:  true,
	TransportNym:  true,
	TransportTor:  true,
}

// ExtractTransport extracts the transport protocol from a multiaddress.
// Returns the transport identifier (e.g., "tcp", "quic", "i2p", "nym") or an error.
func ExtractTransport(addr multiaddr.Multiaddr) (string, error) {
	if addr == nil {
		return "", fmt.Errorf("multiaddr is nil")
	}

	// Parse the multiaddr components
	protocols := addr.Protocols()
	
	// Look for transport protocols in the multiaddr
	// Priority order: application-level transports first, then network transports
	for _, proto := range protocols {
		switch proto.Name {
		case "quic", "quic-v1":
			return TransportQUIC, nil
		case "ws":
			return TransportWS, nil
		case "wss":
			return TransportWSS, nil
		case "onion", "onion3":
			return TransportTor, nil
		case "garlic64", "garlic32": // I2P address formats
			return TransportI2P, nil
		}
	}

	// Check for nym in the string representation (custom protocol)
	addrStr := addr.String()
	if strings.Contains(addrStr, "/nym/") {
		return TransportNym, nil
	}

	// Check for i2p in the string representation (alternative format)
	if strings.Contains(addrStr, "/i2p/") {
		return TransportI2P, nil
	}

	// Fall back to TCP if we find it
	for _, proto := range protocols {
		if proto.Name == "tcp" {
			return TransportTCP, nil
		}
		if proto.Name == "udp" {
			return TransportUDP, nil
		}
	}

	return "", fmt.Errorf("no recognized transport protocol found in multiaddr: %s", addr.String())
}

// ExtractTransportFromString extracts the transport protocol from a multiaddress string.
func ExtractTransportFromString(addrStr string) (string, error) {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return "", fmt.Errorf("invalid multiaddr string: %w", err)
	}
	return ExtractTransport(addr)
}

// FilterMultiaddrsByTransport filters a list of multiaddrs to only include those
// that use one of the allowed transports. If allowedTransports is empty or nil,
// all addresses are returned.
func FilterMultiaddrsByTransport(addrs []multiaddr.Multiaddr, allowedTransports []string) []multiaddr.Multiaddr {
	// If no restrictions, return all addresses
	if len(allowedTransports) == 0 {
		return addrs
	}

	// Build a set of allowed transports for quick lookup
	allowedSet := make(map[string]bool)
	for _, transport := range allowedTransports {
		allowedSet[strings.ToLower(transport)] = true
	}

	// Filter addresses
	filtered := make([]multiaddr.Multiaddr, 0, len(addrs))
	for _, addr := range addrs {
		transport, err := ExtractTransport(addr)
		if err != nil {
			// Skip addresses we can't parse
			continue
		}
		
		if allowedSet[strings.ToLower(transport)] {
			filtered = append(filtered, addr)
		}
	}

	return filtered
}

// FilterMultiaddrStringsByTransport filters a list of multiaddr strings to only include
// those that use one of the allowed transports. If allowedTransports is empty or nil,
// all addresses are returned.
func FilterMultiaddrStringsByTransport(addrStrs []string, allowedTransports []string) []string {
	// If no restrictions, return all addresses
	if len(allowedTransports) == 0 {
		return addrStrs
	}

	// Build a set of allowed transports for quick lookup
	allowedSet := make(map[string]bool)
	for _, transport := range allowedTransports {
		allowedSet[strings.ToLower(transport)] = true
	}

	// Filter addresses
	filtered := make([]string, 0, len(addrStrs))
	for _, addrStr := range addrStrs {
		transport, err := ExtractTransportFromString(addrStr)
		if err != nil {
			// Skip addresses we can't parse
			continue
		}
		
		if allowedSet[strings.ToLower(transport)] {
			filtered = append(filtered, addrStr)
		}
	}

	return filtered
}

// ValidateTransportIdentifier checks if a transport identifier is valid.
func ValidateTransportIdentifier(transport string) bool {
	return ValidTransports[strings.ToLower(transport)]
}

// ValidateTransportIdentifiers checks if all transport identifiers in a list are valid.
// Returns true if all are valid, false otherwise.
func ValidateTransportIdentifiers(transports []string) bool {
	for _, transport := range transports {
		if !ValidateTransportIdentifier(transport) {
			return false
		}
	}
	return true
}

// MatchesTransportRestrictions checks if a multiaddr matches the transport restrictions.
// Returns true if the address uses one of the allowed transports, or if there are no restrictions.
func MatchesTransportRestrictions(addr multiaddr.Multiaddr, allowedTransports []string) bool {
	// No restrictions means all transports are allowed
	if len(allowedTransports) == 0 {
		return true
	}

	transport, err := ExtractTransport(addr)
	if err != nil {
		return false
	}

	// Check if the transport is in the allowed list
	for _, allowed := range allowedTransports {
		if strings.EqualFold(transport, allowed) {
			return true
		}
	}

	return false
}

// MatchesTransportRestrictionsString checks if a multiaddr string matches the transport restrictions.
func MatchesTransportRestrictionsString(addrStr string, allowedTransports []string) bool {
	addr, err := multiaddr.NewMultiaddr(addrStr)
	if err != nil {
		return false
	}
	return MatchesTransportRestrictions(addr, allowedTransports)
}

// GetTransportInfo returns a human-readable description of the transport used by a multiaddr.
func GetTransportInfo(addr multiaddr.Multiaddr) string {
	transport, err := ExtractTransport(addr)
	if err != nil {
		return "unknown"
	}

	switch transport {
	case TransportTCP:
		return "TCP"
	case TransportUDP:
		return "UDP"
	case TransportQUIC:
		return "QUIC"
	case TransportWS:
		return "WebSocket"
	case TransportWSS:
		return "WebSocket Secure"
	case TransportI2P:
		return "I2P"
	case TransportNym:
		return "Nym Mixnet"
	case TransportTor:
		return "Tor"
	default:
		return transport
	}
}

