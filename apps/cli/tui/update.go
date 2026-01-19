package tui

import (
	"banyan-cli/nodeproc"
	"banyan-cli/session"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// Update handles messages
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		return m.handleKeyMsg(msg)

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.cmdInput.Width = msg.Width - 4
		// Initialize viewports if not done
		if m.eventView.Width == 0 {
			m.eventView = newViewport(msg.Width-4, msg.Height-10)
			m.statusView = newViewport(msg.Width-4, msg.Height-10)
			m.peerView = newViewport(msg.Width-4, msg.Height-10)
			m.logView = newViewport(msg.Width-4, msg.Height-10)
			m.instancesView = newViewport(msg.Width-4, msg.Height-10)
			m.serviceView = newViewport(msg.Width-4, msg.Height-10)
			m.helpView = newViewport(msg.Width-4, msg.Height-10)
			// Set initial content for help view (static)
			m.helpView.SetContent(m.renderHelpContent())
		} else {
			m.eventView.Width = msg.Width - 4
			m.eventView.Height = msg.Height - 10
			m.statusView.Width = msg.Width - 4
			m.statusView.Height = msg.Height - 10
			m.peerView.Width = msg.Width - 4
			m.peerView.Height = msg.Height - 10
			m.logView.Width = msg.Width - 4
			m.logView.Height = msg.Height - 10
			m.instancesView.Width = msg.Width - 4
			m.instancesView.Height = msg.Height - 10
			m.serviceView.Width = msg.Width - 4
			m.serviceView.Height = msg.Height - 10
			m.helpView.Width = msg.Width - 4
			m.helpView.Height = msg.Height - 10
		}
		return m, nil

	case connectMsg:
		m.message = fmt.Sprintf("Connecting to %s...", msg.address)
		m.messageTime = time.Now()
		// If instanceIdx is -1, create new instance
		idx := msg.instanceIdx
		if idx < 0 {
			inst := &NodeInstance{Address: msg.address, Connected: false}
			idx = m.AddInstance(inst)
			m.activeInstanceIdx = idx
		}
		return m, doConnect(msg.address, idx)

	case connectedMsg:
		m.executing = false // Stop spinner
		m.errorCount = 0    // Reset error count on successful connection
		m.message = "Connected to node"
		m.messageTime = time.Now()

		// Log WebSocket error if any
		if msg.wsErr != nil {
			m.AddLog(fmt.Sprintf("WebSocket subscription failed: %v", msg.wsErr))
			m.logView.SetContent(m.renderLogsContent())
		}

		// Update or create instance
		idx := msg.instanceIdx
		if idx >= 0 && idx < len(m.instances) {
			m.instances[idx].Client = msg.client
			m.instances[idx].Connected = true
			m.instances[idx].Address = msg.client.BaseURL()
		} else {
			// Create new instance
			inst := &NodeInstance{
				Client:    msg.client,
				Address:   msg.client.BaseURL(),
				Connected: true,
			}
			idx = m.AddInstance(inst)
		}
		m.activeInstanceIdx = idx

		// Log this instance to the logged instances list (historical record)
		m.config.AddLoggedInstance(msg.client.BaseURL(), m.instances[idx].Name)
		m.config.SaveLoggedInstances()

		// Update instances view
		m.instancesView.SetContent(m.renderInstancesContent())

		// Save instances to config for auto-reconnect
		m.SaveInstancesToConfig()

		cmds = append(cmds, fetchStatus(msg.client), fetchPeers(msg.client), fetchServices(msg.client), tick())
		// Always start listening for events (listenForEvents handles disconnected state)
		cmds = append(cmds, listenForEvents(msg.client))
		return m, tea.Batch(cmds...)

	case connectionErrorMsg:
		m.executing = false // Stop spinner
		m.err = msg.err
		m.message = fmt.Sprintf("Connection error: %v", msg.err)
		m.messageTime = time.Now()
		// Mark instance as disconnected if it exists
		if msg.instanceIdx >= 0 && msg.instanceIdx < len(m.instances) {
			m.instances[msg.instanceIdx].Connected = false
			m.SyncInstancesToWebUI()
		}
		// Log the full error
		m.AddLog(fmt.Sprintf("Connection error: %v", msg.err))
		m.logView.SetContent(m.renderLogsContent())
		return m, nil

	case statusUpdateMsg:
		if msg.err != nil {
			m.AddLog(fmt.Sprintf("Status fetch error: %v", msg.err))
			m.logView.SetContent(m.renderLogsContent())
			m.errorCount++
			m.lastErrorTime = time.Now()
			// After 3 consecutive errors, mark instance as disconnected
			if m.errorCount >= 3 {
				inst := m.ActiveInstance()
				if inst != nil && inst.Connected {
					inst.Connected = false
					m.message = "Connection lost to active node"
					m.messageTime = time.Now()
					m.AddLog("Active instance marked as disconnected after repeated failures")
					m.logView.SetContent(m.renderLogsContent())
					m.SaveInstancesToConfig()
				}
			}
			return m, nil
		}
		m.status = msg.status
		m.errorCount = 0 // Reset on success
		m.statusView.SetContent(m.renderStatusContent())
		return m, nil

	case peersUpdateMsg:
		if msg.err != nil {
			// Error already logged by status handler
			return m, nil
		}
		m.peers = msg.peers
		m.peerView.SetContent(m.renderPeersContent())
		return m, nil

	case servicesUpdateMsg:
		if msg.err != nil {
			m.AddLog(fmt.Sprintf("Failed to fetch services: %v", msg.err))
			m.logView.SetContent(m.renderLogsContent())
			return m, nil
		}
		m.services = msg.services
		m.locators = msg.locators
		m.serviceView.SetContent(m.renderServicesContent())
		return m, nil

	case logMsg:
		// Add to logs and update view
		m.AddLog(msg.message)
		m.logView.SetContent(m.renderLogsContent())
		m.logView.GotoBottom()
		return m, nil

	case instanceHealthMsg:
		// Update instance health status
		if msg.instanceIdx >= 0 && msg.instanceIdx < len(m.instances) {
			inst := m.instances[msg.instanceIdx]
			wasConnected := inst.Connected
			inst.Connected = msg.connected
			if wasConnected && !msg.connected {
				m.AddLog(fmt.Sprintf("Instance %d (%s) is now unreachable", msg.instanceIdx+1, inst.Address))
				m.logView.SetContent(m.renderLogsContent())
				m.instancesView.SetContent(m.renderInstancesContent())
				m.SaveInstancesToConfig()
			} else if !wasConnected && msg.connected {
				m.AddLog(fmt.Sprintf("Instance %d (%s) is now reachable", msg.instanceIdx+1, inst.Address))
				m.logView.SetContent(m.renderLogsContent())
				m.instancesView.SetContent(m.renderInstancesContent())
				// If reconnected, subscribe to events
				if inst.Client != nil {
					inst.Client.SubscribeEvents()
				}
				m.SaveInstancesToConfig()
			}
		}
		return m, nil

	case healthCheckMsg:
		// Check health of all non-active instances
		for i, inst := range m.instances {
			if i != m.activeInstanceIdx && inst.Client != nil {
				cmds = append(cmds, checkInstanceHealth(inst, i))
			}
		}
		return m, tea.Batch(cmds...)

	case instanceSelectedMsg:
		// User switched to a different instance - refresh all data
		// Set the active index explicitly (in case it wasn't persisted through the Bubble Tea cycle)
		m.activeInstanceIdx = msg.idx
		m.AddLog(fmt.Sprintf("DEBUG instanceSelectedMsg received: idx=%d, address=%s", msg.idx, msg.address))

		m.message = fmt.Sprintf("Switched to instance %d: %s", msg.idx+1, msg.address)
		m.messageTime = time.Now()

		// Clear old data
		m.status = nil
		m.peers = nil
		m.events = nil
		m.services = nil
		m.locators = nil
		m.errorCount = 0

		// Update all views to show cleared/loading state
		m.statusView.SetContent(m.renderStatusContent())
		m.peerView.SetContent(m.renderPeersContent())
		m.eventView.SetContent(m.renderEventsContent())
		m.serviceView.SetContent(m.renderServicesContent())
		m.instancesView.SetContent(m.renderInstancesContent())

		// Fetch fresh data from the new active instance
		c := m.ActiveClient()
		if c != nil {
			cmds = append(cmds, fetchStatus(c), fetchPeers(c), fetchServices(c), listenForEvents(c))
		}

		// Save to config
		m.SaveInstancesToConfig()
		return m, tea.Batch(cmds...)

	case instanceRemovedMsg:
		// Remove disconnected instance (1-based index)
		idx := msg.idx - 1
		if idx >= 0 && idx < len(m.instances) {
			m.instances = append(m.instances[:idx], m.instances[idx+1:]...)
			// Adjust active index
			if m.activeInstanceIdx >= len(m.instances) {
				m.activeInstanceIdx = len(m.instances) - 1
			}
			if m.activeInstanceIdx < 0 {
				m.activeInstanceIdx = 0
			}
			// Update views
			m.instancesView.SetContent(m.renderInstancesContent())
			m.message = fmt.Sprintf("Removed disconnected instance %d", msg.idx)
			m.messageTime = time.Now()
			// Save to config
			m.SaveInstancesToConfig()
		}
		return m, nil

	case reconnectLoggedMsg:
		// User wants to reconnect to a logged instance
		m.message = fmt.Sprintf("Reconnecting to logged instance L%d: %s...", msg.loggedIdx, msg.address)
		m.messageTime = time.Now()

		// Create a new instance entry for the reconnection attempt
		inst := &NodeInstance{
			Address:   msg.address,
			Name:      msg.name,
			Connected: false,
		}
		idx := m.AddInstance(inst)
		m.activeInstanceIdx = idx

		// Update views
		m.instancesView.SetContent(m.renderInstancesContent())

		// Attempt connection
		return m, doConnect(msg.address, idx)

	case eventMsg:
		c := m.ActiveClient()
		// Handle special event types for debugging
		switch msg.event.Type {
		case "websocket_disconnected":
			m.message = "WebSocket disconnected - connection may be lost"
			m.messageTime = time.Now()
			m.AddLog("WebSocket disconnected")
			m.logView.SetContent(m.renderLogsContent())
			// Mark active instance as disconnected
			inst := m.ActiveInstance()
			if inst != nil {
				inst.Connected = false
				m.SaveInstancesToConfig()
			}
			return m, nil
		case "websocket_error":
			if data, ok := msg.event.Data.(map[string]string); ok {
				m.AddLog(fmt.Sprintf("WebSocket error: %s", data["error"]))
			} else {
				m.AddLog(fmt.Sprintf("WebSocket error: %v", msg.event.Data))
			}
			m.logView.SetContent(m.renderLogsContent())
			// Continue listening
			if c != nil && c.IsConnected() {
				cmds = append(cmds, listenForEvents(c))
			}
			return m, tea.Batch(cmds...)
		case "parse_error":
			if data, ok := msg.event.Data.(map[string]string); ok {
				m.AddLog(fmt.Sprintf("Event parse error: %s\nRaw message: %s", data["error"], data["message"]))
			} else {
				m.AddLog(fmt.Sprintf("Event parse error: %v", msg.event.Data))
			}
			m.logView.SetContent(m.renderLogsContent())
			// Continue listening
			if c != nil && c.IsConnected() {
				cmds = append(cmds, listenForEvents(c))
			}
			return m, tea.Batch(cmds...)
		}
		// Normal event - add to events list
		m.events = append(m.events, msg.event)
		if len(m.events) > m.maxEvents {
			m.events = m.events[1:]
		}
		m.eventView.SetContent(m.renderEventsContent())
		m.eventView.GotoBottom()
		// Continue listening for events only if still connected
		if c != nil && c.IsConnected() {
			cmds = append(cmds, listenForEvents(c))
		}
		return m, tea.Batch(cmds...)

	case nodeStartedMsg:
		// Create or update instance for started node
		inst := &NodeInstance{
			Address:    msg.url,
			NodeProc:   msg.nodeProc,
			KillOnExit: msg.killOnExit,
			Connected:  false,
		}
		idx := m.AddInstance(inst)
		m.activeInstanceIdx = idx

		if msg.killOnExit {
			m.message = fmt.Sprintf("Node started at %s (subprocess mode), connecting...", msg.url)
		} else {
			m.message = fmt.Sprintf("Node started at %s, connecting...", msg.url)
			// Save last node address for auto-reconnect (only if not subprocess mode)
			m.config.SetLastNodeAddress(msg.url)
		}
		m.messageTime = time.Now()
		return m, doConnect(msg.url, idx)

	case nodeStartErrorMsg:
		m.executing = false // Stop spinner
		m.err = msg.err
		m.message = fmt.Sprintf("Failed to start node: %v", msg.err)
		m.messageTime = time.Now()
		return m, nil

	case commandResultMsg:
		// Stop spinner when command completes
		m.executing = false
		if msg.err != nil {
			m.message = fmt.Sprintf("Error: %v", msg.err)
			m.err = msg.err
			m.errorCount++
			m.lastErrorTime = time.Now()
		} else {
			m.message = msg.result
			m.err = nil
			m.errorCount = 0 // Reset on success
		}
		m.messageTime = time.Now()
		// Don't auto-refresh on error to prevent loops
		c := m.ActiveClient()
		if m.IsConnected() && c != nil && msg.err == nil {
			cmds = append(cmds, fetchStatus(c), fetchPeers(c))
		}
		return m, tea.Batch(cmds...)

	case spinnerTickMsg:
		// Animate spinner while executing
		if m.executing {
			m.spinnerFrame++
			return m, spinnerTick()
		}
		return m, nil

	case animTickMsg:
		// Animate title bar
		m.animFrame++
		return m, animTick()

	case sessionCommandMsg:
		// Handle command from WebUI via session
		return m.handleSessionCommand(msg.cmd)

	case tickMsg:
		// Implement exponential backoff on repeated errors
		if m.errorCount > 5 {
			// Too many errors, slow down polling
			backoff := time.Duration(m.errorCount) * time.Second
			if backoff > 30*time.Second {
				backoff = 30 * time.Second
			}
			if time.Since(m.lastErrorTime) < backoff {
				return m, tick() // Skip this tick
			}
		}
		c := m.ActiveClient()
		if m.IsConnected() && c != nil {
			cmds = append(cmds, fetchStatus(c), fetchPeers(c), tick())
			// Also check health of other instances periodically
			cmds = append(cmds, triggerHealthCheck())
			return m, tea.Batch(cmds...)
		}
		return m, tick()
	}

	// Update text input
	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(msg)
	if cmd != nil {
		cmds = append(cmds, cmd)
	}

	return m, tea.Batch(cmds...)
}

func (m Model) handleKeyMsg(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "ctrl+c", "ctrl+q":
		// Mark TUI as inactive so web handlers know not to wait
		session.Get().SetTUIActive(false)
		// Save instances before exiting
		m.SaveInstancesToConfig()
		// Kill all subprocess instances
		for _, inst := range m.instances {
			if inst.KillOnExit && inst.NodeProc != nil {
				inst.NodeProc.Stop()
			}
			if inst.Client != nil {
				inst.Client.Close()
			}
		}
		return m, tea.Quit

	case "tab":
		m.currentView = (m.currentView + 1) % 7
		return m, nil

	case "shift+tab":
		m.currentView = (m.currentView + 6) % 7
		return m, nil

	// Use Ctrl+number to switch tabs (so plain numbers work in command input)
	case "ctrl+1":
		m.currentView = ViewStatus
		return m, nil
	case "ctrl+2":
		m.currentView = ViewEvents
		return m, nil
	case "ctrl+3":
		m.currentView = ViewPeers
		return m, nil
	case "ctrl+4":
		m.currentView = ViewServices
		return m, nil
	case "ctrl+5":
		m.currentView = ViewInstances
		return m, nil
	case "ctrl+6":
		m.currentView = ViewLogs
		return m, nil
	case "ctrl+7", "?":
		m.currentView = ViewHelp
		return m, nil

	case "enter":
		cmd := m.cmdInput.Value()
		if cmd != "" {
			m.cmdHistory = append(m.cmdHistory, cmd)
			m.historyIdx = len(m.cmdHistory)
			m.cmdInput.SetValue("")
			// Start spinner while executing command
			m.executing = true
			m.spinnerFrame = 0
			m.spinnerTick = time.Now()
			return m, tea.Batch(m.executeCommand(cmd), spinnerTick())
		}
		return m, nil

	case "up":
		if len(m.cmdHistory) > 0 && m.historyIdx > 0 {
			m.historyIdx--
			m.cmdInput.SetValue(m.cmdHistory[m.historyIdx])
		}
		return m, nil

	case "down":
		if m.historyIdx < len(m.cmdHistory)-1 {
			m.historyIdx++
			m.cmdInput.SetValue(m.cmdHistory[m.historyIdx])
		} else {
			m.historyIdx = len(m.cmdHistory)
			m.cmdInput.SetValue("")
		}
		return m, nil

	case "pgup":
		// Scroll current view up by half a page
		switch m.currentView {
		case ViewStatus:
			m.statusView.ViewUp()
		case ViewEvents:
			m.eventView.ViewUp()
		case ViewPeers:
			m.peerView.ViewUp()
		case ViewServices:
			m.serviceView.ViewUp()
		case ViewInstances:
			m.instancesView.ViewUp()
		case ViewLogs:
			m.logView.ViewUp()
		case ViewHelp:
			m.helpView.ViewUp()
		}
		return m, nil

	case "pgdown":
		// Scroll current view down by half a page
		switch m.currentView {
		case ViewStatus:
			m.statusView.ViewDown()
		case ViewEvents:
			m.eventView.ViewDown()
		case ViewPeers:
			m.peerView.ViewDown()
		case ViewServices:
			m.serviceView.ViewDown()
		case ViewInstances:
			m.instancesView.ViewDown()
		case ViewLogs:
			m.logView.ViewDown()
		case ViewHelp:
			m.helpView.ViewDown()
		}
		return m, nil

	case "home":
		// Scroll to top
		switch m.currentView {
		case ViewStatus:
			m.statusView.GotoTop()
		case ViewEvents:
			m.eventView.GotoTop()
		case ViewPeers:
			m.peerView.GotoTop()
		case ViewServices:
			m.serviceView.GotoTop()
		case ViewInstances:
			m.instancesView.GotoTop()
		case ViewLogs:
			m.logView.GotoTop()
		case ViewHelp:
			m.helpView.GotoTop()
		}
		return m, nil

	case "end":
		// Scroll to bottom
		switch m.currentView {
		case ViewStatus:
			m.statusView.GotoBottom()
		case ViewEvents:
			m.eventView.GotoBottom()
		case ViewPeers:
			m.peerView.GotoBottom()
		case ViewServices:
			m.serviceView.GotoBottom()
		case ViewInstances:
			m.instancesView.GotoBottom()
		case ViewLogs:
			m.logView.GotoBottom()
		case ViewHelp:
			m.helpView.GotoBottom()
		}
		return m, nil
	}

	var cmd tea.Cmd
	m.cmdInput, cmd = m.cmdInput.Update(msg)
	return m, cmd
}

func newViewport(width, height int) viewport.Model {
	vp := viewport.New(width, height)
	vp.SetContent("")
	return vp
}

func (m Model) cmdStartNode(args []string) tea.Cmd {
	// Parse start command arguments
	var nodePath, configPath, nodeArgs string
	killOnExit := false // --subprocess flag

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--node-path", "-p":
			if i+1 < len(args) {
				nodePath = args[i+1]
				i++
			}
		case "--config", "-c":
			if i+1 < len(args) {
				configPath = args[i+1]
				i++
			}
		case "--args", "-a":
			if i+1 < len(args) {
				nodeArgs = args[i+1]
				i++
			}
		case "--subprocess", "-s":
			killOnExit = true
		default:
			// First positional arg is node path
			if nodePath == "" && !strings.HasPrefix(args[i], "-") {
				nodePath = args[i]
			}
		}
	}

	return func() tea.Msg {
		// Determine node executable path
		execPath := nodePath
		if execPath == "" {
			// Try config
			execPath = m.config.NodeExecutable
		}
		if execPath == "" {
			// Auto-detect: look in same directory as CLI executable
			execPath = findNodeExecutable()
		}
		if execPath == "" {
			return nodeStartErrorMsg{err: fmt.Errorf("node executable not found. Use: start [path] or start --node-path <path>")}
		}

		// Determine config path
		cfgPath := configPath
		if cfgPath == "" {
			cfgPath = m.config.NodeConfigPath
		}

		// Parse extra args
		var extraArgs []string
		if nodeArgs != "" {
			extraArgs = parseArgs(nodeArgs)
		}

		np := nodeproc.New(execPath, cfgPath, extraArgs)
		if err := np.Start(); err != nil {
			return nodeStartErrorMsg{err: err}
		}

		url, err := np.WaitForManagementURL(30 * time.Second)
		if err != nil {
			np.Stop()
			return nodeStartErrorMsg{err: err}
		}

		return nodeStartedMsg{url: url, nodeProc: np, killOnExit: killOnExit}
	}
}

// findNodeExecutable looks for any executable starting with "banyan" in the same directory as the CLI
func findNodeExecutable() string {
	cliPath, err := os.Executable()
	if err != nil {
		return ""
	}
	cliPath, _ = filepath.EvalSymlinks(cliPath) // Resolve symlinks
	dir := filepath.Dir(cliPath)
	cliName := filepath.Base(cliPath)

	// Look for any file starting with "banyan" that isn't the CLI itself
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()

		// Skip if it's the CLI executable itself
		if name == cliName {
			continue
		}

		// Check if it starts with "banyan" (case-insensitive)
		nameLower := strings.ToLower(name)
		if !strings.HasPrefix(nameLower, "banyan") {
			continue
		}

		// On Windows, must end with .exe
		if runtime.GOOS == "windows" && !strings.HasSuffix(nameLower, ".exe") {
			continue
		}

		// Found a candidate
		candidate := filepath.Join(dir, name)

		// On Unix, check if it's executable
		if runtime.GOOS != "windows" {
			info, err := entry.Info()
			if err != nil {
				continue
			}
			// Check if any execute bit is set
			if info.Mode()&0111 == 0 {
				continue
			}
		}

		return candidate
	}
	return ""
}

// parseArgs splits a string into arguments, respecting quotes
func parseArgs(s string) []string {
	var args []string
	var current strings.Builder
	inQuote := false
	quoteChar := rune(0)

	for _, r := range s {
		switch {
		case (r == '"' || r == '\'') && !inQuote:
			inQuote = true
			quoteChar = r
		case r == quoteChar && inQuote:
			inQuote = false
			quoteChar = 0
		case r == ' ' && !inQuote:
			if current.Len() > 0 {
				args = append(args, current.String())
				current.Reset()
			}
		default:
			current.WriteRune(r)
		}
	}
	if current.Len() > 0 {
		args = append(args, current.String())
	}
	return args
}

// handleSessionCommand handles commands from WebUI via session
func (m Model) handleSessionCommand(cmd session.Command) (tea.Model, tea.Cmd) {
	sess := session.Get()

	switch cmd.Action {
	case "select":
		idx := cmd.Index - 1 // Convert to 0-based
		if idx < 0 || idx >= len(m.instances) {
			sess.SendResult(session.CommandResult{
				Success: false,
				Message: fmt.Sprintf("invalid instance index (1-%d)", len(m.instances)),
			})
			return m, listenForSessionCommands()
		}

		inst := m.instances[idx]
		if !inst.Connected {
			// Remove disconnected instance
			m.instances = append(m.instances[:idx], m.instances[idx+1:]...)
			if m.activeInstanceIdx >= len(m.instances) {
				m.activeInstanceIdx = len(m.instances) - 1
			}
			m.SaveInstancesToConfig()
			m.SyncInstancesToWebUI()
			sess.SendResult(session.CommandResult{
				Success: true,
				Removed: true,
				Message: "removed disconnected instance",
			})
			return m, listenForSessionCommands()
		}

		// Select the instance
		m.activeInstanceIdx = idx
		m.SaveInstancesToConfig()
		m.SyncInstancesToWebUI()
		sess.SendResult(session.CommandResult{
			Success: true,
			Address: inst.Address,
		})
		// Refresh data for new active instance
		c := m.ActiveClient()
		if c != nil {
			return m, tea.Batch(listenForSessionCommands(), fetchStatus(c), fetchPeers(c))
		}
		return m, listenForSessionCommands()

	case "stop":
		idx := cmd.Index - 1
		if cmd.Index == 0 {
			idx = m.activeInstanceIdx
		}
		if idx < 0 || idx >= len(m.instances) {
			sess.SendResult(session.CommandResult{
				Success: false,
				Message: "invalid instance index",
			})
			return m, listenForSessionCommands()
		}

		inst := m.instances[idx]
		// Stop node process if available
		if inst.NodeProc != nil {
			inst.NodeProc.Stop()
			inst.NodeProc = nil
		} else if inst.Client != nil {
			inst.Client.Shutdown()
		}
		if inst.Client != nil {
			inst.Client.Close()
		}
		inst.Connected = false
		m.SaveInstancesToConfig()
		m.SyncInstancesToWebUI()
		sess.SendResult(session.CommandResult{
			Success: true,
			Message: fmt.Sprintf("stopped instance %d", idx+1),
		})
		return m, listenForSessionCommands()

	default:
		sess.SendResult(session.CommandResult{
			Success: false,
			Message: fmt.Sprintf("unknown command: %s", cmd.Action),
		})
		return m, listenForSessionCommands()
	}
}
