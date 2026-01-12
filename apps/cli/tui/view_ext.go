package tui

import (
	"fmt"
	"strings"
	"time"
)

func (m Model) renderServicesContent() string {
	if !m.IsConnected() {
		return "Not connected. Use 'connect <address>' to connect to a node."
	}

	var b strings.Builder
	b.WriteString("Service Management\n")
	b.WriteString("==================\n\n")

	// Active Beacons
	b.WriteString("Active Beacons:\n")
	b.WriteString(strings.Repeat("─", 50) + "\n")
	if m.services != nil {
		if beacons, ok := m.services["beacons"].([]interface{}); ok && len(beacons) > 0 {
			for _, beacon := range beacons {
				if info, ok := beacon.(map[string]interface{}); ok {
					alias := ""
					if a, ok := info["alias"].(string); ok && a != "" {
						alias = a
					}
					keyHash := ""
					if kh, ok := info["serviceKeyHash"].(string); ok {
						keyHash = kh
					}
					mode := ""
					if m, ok := info["mode"].(string); ok {
						mode = m
					}
					peerCount := 0
					if pc, ok := info["peerCount"].(float64); ok {
						peerCount = int(pc)
					}
					if alias != "" {
						b.WriteString(fmt.Sprintf("  • %s [%s] mode=%s peers=%d\n", alias, keyHash, mode, peerCount))
					} else {
						b.WriteString(fmt.Sprintf("  • %s mode=%s peers=%d\n", keyHash, mode, peerCount))
					}
				}
			}
		} else {
			b.WriteString("  (none)\n")
		}
	} else {
		b.WriteString("  Loading...\n")
	}
	b.WriteString("\n")

	// Active Locators
	b.WriteString("Active Locators:\n")
	b.WriteString(strings.Repeat("─", 50) + "\n")
	if m.locators != nil {
		if items, ok := m.locators["locators"].([]interface{}); ok && len(items) > 0 {
			for _, item := range items {
				if info, ok := item.(map[string]interface{}); ok {
					keyHash := ""
					if kh, ok := info["serviceKeyHash"].(string); ok {
						keyHash = kh
					}
					peers := []interface{}{}
					if p, ok := info["peers"].([]interface{}); ok {
						peers = p
					}
					b.WriteString(fmt.Sprintf("  • %s (%d peers)\n", keyHash, len(peers)))
					for _, peer := range peers {
						if peerID, ok := peer.(string); ok {
							shortID := peerID
							if len(shortID) > 20 {
								shortID = shortID[:8] + "..." + shortID[len(shortID)-8:]
							}
							b.WriteString(fmt.Sprintf("      → %s\n", shortID))
						}
					}
				}
			}
		} else {
			b.WriteString("  (none)\n")
		}
	} else {
		b.WriteString("  Loading...\n")
	}
	b.WriteString("\n")

	// Commands
	b.WriteString("Commands:\n")
	b.WriteString("  services        - Refresh services list\n")
	b.WriteString("  figs            - List service figs\n")
	b.WriteString("  find <key>      - Find service by key or alias\n")
	b.WriteString("  serve start <f> - Start service beacon with fig file\n")
	b.WriteString("  serve stop [h]  - Stop service beacon (optional hash)\n")

	return b.String()
}

func (m Model) renderInstancesContent() string {
	var b strings.Builder
	b.WriteString("Node Instances\n")
	b.WriteString("==============\n\n")

	b.WriteString("Commands:\n")
	b.WriteString("  connect <addr>   - Connect to a new node instance\n")
	b.WriteString("  select <n>       - Switch to instance number n\n")
	b.WriteString("  stop [n]         - Stop instance n (or active if omitted)\n")
	b.WriteString("  disconnect [n]   - Disconnect from instance n\n")
	b.WriteString("  reconnect <n>    - Reconnect to logged instance n\n")
	b.WriteString("  clear logged     - Clear logged instances history\n")
	b.WriteString("\n")

	// Active instances
	b.WriteString("Active Instances:\n")
	b.WriteString(strings.Repeat("─", 60) + "\n")

	if len(m.instances) == 0 {
		b.WriteString("  (none) - Use 'connect <address>' or 'start' to add one.\n")
	} else {
		for i, inst := range m.instances {
			marker := "  "
			if i == m.activeInstanceIdx {
				marker = "► "
			}

			status := "disconnected"
			statusStyle := "○"
			if inst.Connected {
				status = "connected"
				statusStyle = "●"
			}

			subprocess := ""
			if inst.KillOnExit {
				subprocess = " [subprocess]"
			}

			name := inst.Address
			if inst.Name != "" {
				name = inst.Name + " (" + inst.Address + ")"
			}

			b.WriteString(fmt.Sprintf("%s[%d] %s %s%s - %s\n", marker, i+1, statusStyle, name, subprocess, status))
		}
	}

	b.WriteString(strings.Repeat("─", 60) + "\n")
	b.WriteString(fmt.Sprintf("Total: %d active instance(s)\n", len(m.instances)))
	b.WriteString("\n")

	// Logged instances (historical)
	loggedInstances := m.config.GetLoggedInstances()
	b.WriteString("Logged Instances (History):\n")
	b.WriteString(strings.Repeat("─", 60) + "\n")

	if len(loggedInstances) == 0 {
		b.WriteString("  (none) - Instances you connect to will appear here.\n")
	} else {
		for i, inst := range loggedInstances {
			name := inst.Address
			if inst.Name != "" {
				name = inst.Name + " (" + inst.Address + ")"
			}
			lastSeen := ""
			if inst.LastSeen != "" {
				lastSeen = fmt.Sprintf(" [last: %s]", formatTimestamp(inst.LastSeen))
			}
			b.WriteString(fmt.Sprintf("  [L%d] %s%s\n", i+1, name, lastSeen))
		}
	}

	b.WriteString(strings.Repeat("─", 60) + "\n")
	b.WriteString(fmt.Sprintf("Total: %d logged instance(s)\n", len(loggedInstances)))
	b.WriteString("\nTip: Use 'reconnect L<n>' to reconnect to a logged instance.\n")
	return b.String()
}

// formatTimestamp formats an RFC3339 timestamp for display
func formatTimestamp(ts string) string {
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return ts
	}
	return t.Format("Jan 2 15:04")
}

func (m Model) renderHelpContent() string {
	return `
Banyan CLI Help
===============

Navigation:
  Tab / Shift+Tab    - Switch between views
  Ctrl+1 to Ctrl+7   - Jump to specific view
  Up/Down            - Navigate command history
  PgUp/PgDown        - Scroll content up/down
  Home/End           - Scroll to top/bottom
  Ctrl+Q             - Quit

Instance Commands (Multi-Node Support):
  connect <address>     - Connect to a new node instance
  select <n>            - Switch to instance number n
  stop [n]              - Stop/disconnect instance n (or active)
  disconnect [n]        - Disconnect from instance n
  reconnect <L#>        - Reconnect to logged instance (e.g., reconnect L1)

Node Commands:
  start [path] [opts]   - Start node (auto-detects in same dir as CLI)
                          Options: --config/-c <file>, --args/-a "<args>"
                                   --subprocess/-s (kill node on CLI exit)
  status                - Refresh node status

Note: The CLI remembers the last connected node and auto-reconnects on startup.

Peer Commands:
  peers              - List all connected peers
  peer connect <id>  - Connect to a peer by ID or alias
  add <id> <addr>    - Add a peer with multiaddr [--connect]

Service Commands:
  services           - List configured services
  figs               - List service figs
  find <key|alias>   - Find a service
  serve start <file> - Start service beacon
  serve stop [hash]  - Stop service beacon

Proxy Commands:
  proxy peer <id> <path> [method]    - Proxy request through peer
  proxy service <key> <path> [method] - Proxy request through service

Route Commands:
  route list         - List configured routes
  route add <p> <t>  - Add a route (path -> target)

Other Commands:
  clear              - Clear event log
  clear logs         - Clear debug logs
  clear logged       - Clear logged instances history
  help               - Show this help

Views:
  [1] Status    - Node status and statistics
  [2] Events    - Live WebSocket event log
  [3] Peers     - Connected peers list
  [4] Services  - Service management info
  [5] Instances - Connected node instances
  [6] Logs      - Debug logs and error messages
  [7] Help      - This help screen
`
}

func (m Model) renderLogsContent() string {
	if len(m.logs) == 0 {
		return "No logs yet. Errors and debug information will appear here.\n\nTip: Use 'clear logs' to clear the log buffer."
	}

	var b strings.Builder
	b.WriteString("Debug Logs (")
	b.WriteString(string(rune('0' + len(m.logs)/100%10)))
	b.WriteString(string(rune('0' + len(m.logs)/10%10)))
	b.WriteString(string(rune('0' + len(m.logs)%10)))
	b.WriteString(" entries)\n")
	b.WriteString(strings.Repeat("─", 40))
	b.WriteString("\n\n")

	// Word wrap each log entry
	width := m.width - 8
	if width < 40 {
		width = 40
	}

	for _, log := range m.logs {
		wrapped := wordWrap(log, width)
		b.WriteString(wrapped)
		b.WriteString("\n")
	}

	return b.String()
}

// wordWrap wraps text at the specified width
func wordWrap(text string, width int) string {
	if len(text) <= width {
		return text
	}

	var result strings.Builder
	var lineLen int

	words := strings.Fields(text)
	for i, word := range words {
		if lineLen+len(word)+1 > width && lineLen > 0 {
			result.WriteString("\n    ") // Indent continuation lines
			lineLen = 4
		} else if i > 0 {
			result.WriteString(" ")
			lineLen++
		}
		result.WriteString(word)
		lineLen += len(word)
	}

	return result.String()
}
