package tui

import (
	"banyan-cli/client"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// executeCommand parses and executes a command
func (m Model) executeCommand(input string) tea.Cmd {
	parts := strings.Fields(input)
	if len(parts) == 0 {
		return nil
	}

	cmd := strings.ToLower(parts[0])
	args := parts[1:]

	switch cmd {
	case "help", "h":
		return m.cmdHelp()
	case "connect":
		return m.cmdConnect(args)
	case "start":
		return m.cmdStartNode(args)
	case "stop":
		return m.cmdStopNode(args)
	case "select":
		return m.cmdSelect(args)
	case "disconnect":
		return m.cmdDisconnect(args)
	case "status":
		return m.cmdStatus()
	case "peers":
		return m.cmdPeers()
	case "peer":
		return m.cmdPeer(args)
	case "add":
		return m.cmdAddPeer(args)
	case "find":
		return m.cmdFind(args)
	case "services":
		return m.cmdServices()
	case "figs":
		return m.cmdFigs()
	case "serve":
		return m.cmdServe(args)
	case "proxy":
		return m.cmdProxy(args)
	case "route":
		return m.cmdRoute(args)
	case "reconnect":
		return m.cmdReconnect(args)
	case "clear":
		if len(args) > 0 && args[0] == "logs" {
			m.logs = make([]string, 0)
			m.logView.SetContent("")
			return func() tea.Msg { return commandResultMsg{result: "Logs cleared"} }
		}
		if len(args) > 0 && args[0] == "logged" {
			m.config.ClearLoggedInstances()
			m.config.SaveLoggedInstances()
			m.instancesView.SetContent(m.renderInstancesContent())
			m.SyncLoggedInstancesToWebUI()
			return func() tea.Msg { return commandResultMsg{result: "Logged instances cleared"} }
		}
		m.events = make([]client.Event, 0)
		m.eventView.SetContent("")
		return func() tea.Msg { return commandResultMsg{result: "Events cleared"} }
	default:
		return func() tea.Msg {
			return commandResultMsg{result: "", err: fmt.Errorf("unknown command: %s (type 'help' for available commands)", cmd)}
		}
	}
}

func (m Model) cmdHelp() tea.Cmd {
	return func() tea.Msg {
		m.currentView = ViewHelp
		return commandResultMsg{result: "Showing help view (press Tab to navigate)"}
	}
}

func (m Model) cmdConnect(args []string) tea.Cmd {
	if len(args) == 0 {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("usage: connect <address>")}
		}
	}
	address := args[0]
	if !strings.HasPrefix(address, "http") {
		address = "http://" + address
	}
	return func() tea.Msg {
		return connectMsg{address: address}
	}
}

func (m *Model) cmdStopNode(args []string) tea.Cmd {
	// Stop instance by index, or active instance if no index given
	idx := m.activeInstanceIdx
	if len(args) > 0 {
		n := 0
		fmt.Sscanf(args[0], "%d", &n)
		idx = n - 1 // User uses 1-based index
	}

	if idx < 0 || idx >= len(m.instances) {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("invalid instance index")} }
	}

	inst := m.instances[idx]
	return func() tea.Msg {
		var result string

		// Try to stop via NodeProc if available (subprocess mode)
		if inst.NodeProc != nil {
			if err := inst.NodeProc.Stop(); err != nil {
				return commandResultMsg{err: fmt.Errorf("failed to stop subprocess: %w", err)}
			}
			result = fmt.Sprintf("Instance %d stopped (subprocess killed)", idx+1)
		} else if inst.Client != nil {
			// Remote node - try to call shutdown endpoint
			if err := inst.Client.Shutdown(); err != nil {
				// Shutdown failed - just disconnect instead
				inst.Client.Close()
				inst.Connected = false
				return commandResultMsg{result: fmt.Sprintf("Instance %d disconnected (remote shutdown not available: %v)", idx+1, err)}
			}
			result = fmt.Sprintf("Instance %d shutdown requested", idx+1)
		}

		if inst.Client != nil {
			inst.Client.Close()
		}
		inst.Connected = false
		return commandResultMsg{result: result}
	}
}

func (m *Model) cmdSelect(args []string) tea.Cmd {
	if len(args) == 0 {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: select <instance_number>")} }
	}

	n := 0
	fmt.Sscanf(args[0], "%d", &n)
	idx := n - 1 // User uses 1-based index

	if idx < 0 || idx >= len(m.instances) {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("invalid instance index (1-%d)", len(m.instances))}
		}
	}

	inst := m.instances[idx]
	// Debug: Log instance state
	m.AddLog(fmt.Sprintf("DEBUG cmdSelect: idx=%d, inst.Connected=%v, inst.Address=%s", idx, inst.Connected, inst.Address))

	// If selecting a disconnected instance, send removal message
	if !inst.Connected {
		return func() tea.Msg { return instanceRemovedMsg{idx: n} }
	}

	// Return instanceSelectedMsg to trigger data refresh
	return func() tea.Msg {
		return instanceSelectedMsg{idx: idx, address: inst.Address}
	}
}

func (m *Model) cmdDisconnect(args []string) tea.Cmd {
	idx := m.activeInstanceIdx
	if len(args) > 0 {
		n := 0
		fmt.Sscanf(args[0], "%d", &n)
		idx = n - 1
	}

	if idx < 0 || idx >= len(m.instances) {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("invalid instance index")} }
	}

	inst := m.instances[idx]
	return func() tea.Msg {
		if inst.Client != nil {
			inst.Client.Close()
		}
		inst.Connected = false
		return commandResultMsg{result: fmt.Sprintf("Disconnected from instance %d", idx+1)}
	}
}

func (m Model) cmdStatus() tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	return fetchStatus(c)
}

func (m Model) cmdPeers() tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	return fetchPeers(c)
}

func (m Model) cmdPeer(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	if len(args) == 0 {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: peer <connect> <peerID|alias>")} }
	}

	action := strings.ToLower(args[0])
	switch action {
	case "connect":
		if len(args) < 2 {
			return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: peer connect <peerID|alias>")} }
		}
		peerID := args[1]
		return func() tea.Msg {
			err := c.ConnectToPeer(peerID)
			if err != nil {
				return commandResultMsg{err: err}
			}
			return commandResultMsg{result: fmt.Sprintf("Connected to peer: %s", peerID)}
		}
	default:
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("unknown peer action: %s", action)} }
	}
}

func (m Model) cmdAddPeer(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	if len(args) < 2 {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("usage: add <peerID> <multiaddr> [--connect]")}
		}
	}

	peerID := args[0]
	addr := args[1]
	shouldConnect := len(args) > 2 && args[2] == "--connect"

	return func() tea.Msg {
		req := client.AddPeerRequest{
			PeerID:    peerID,
			Addresses: []string{addr},
			Connect:   shouldConnect,
		}
		_, err := c.AddTrackedPeer(req)
		if err != nil {
			return commandResultMsg{err: err}
		}
		return commandResultMsg{result: fmt.Sprintf("Added peer: %s", peerID)}
	}
}

func (m Model) cmdFind(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	if len(args) == 0 {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: find <serviceKey|alias>")} }
	}

	keyOrAlias := args[0]
	return func() tea.Msg {
		var req client.FindRequest
		if len(keyOrAlias) <= 8 {
			req.Alias = keyOrAlias
		} else {
			req.ServiceKey = keyOrAlias
		}
		resp, err := c.FindService(req)
		if err != nil {
			return commandResultMsg{err: err}
		}
		return commandResultMsg{result: fmt.Sprintf("Service found: %v", resp)}
	}
}

func (m *Model) cmdReconnect(args []string) tea.Cmd {
	if len(args) == 0 {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("usage: reconnect <n> (use L prefix for logged instances, e.g., reconnect L1)")}
		}
	}

	arg := args[0]
	loggedInstances := m.config.GetLoggedInstances()

	// Check if it's a logged instance reference (L1, L2, etc.)
	if strings.HasPrefix(strings.ToUpper(arg), "L") {
		idxStr := strings.TrimPrefix(strings.ToUpper(arg), "L")
		n := 0
		fmt.Sscanf(idxStr, "%d", &n)
		idx := n - 1 // Convert to 0-based

		if idx < 0 || idx >= len(loggedInstances) {
			return func() tea.Msg {
				return commandResultMsg{err: fmt.Errorf("invalid logged instance index (L1-L%d)", len(loggedInstances))}
			}
		}

		address := loggedInstances[idx].Address
		name := loggedInstances[idx].Name
		if !strings.HasPrefix(address, "http") {
			address = "http://" + address
		}

		// Return a reconnect message that will trigger connection
		return func() tea.Msg {
			return reconnectLoggedMsg{address: address, name: name, loggedIdx: n}
		}
	}

	// Otherwise, treat it as a regular instance number
	n := 0
	fmt.Sscanf(arg, "%d", &n)
	idx := n - 1

	if idx < 0 || idx >= len(m.instances) {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("invalid instance index (1-%d), use L prefix for logged instances", len(m.instances))}
		}
	}

	inst := m.instances[idx]
	if inst.Connected {
		return func() tea.Msg {
			return commandResultMsg{result: fmt.Sprintf("Instance %d is already connected", n)}
		}
	}

	// Try to reconnect to the disconnected instance
	return func() tea.Msg {
		return connectMsg{address: inst.Address, instanceIdx: idx}
	}
}
