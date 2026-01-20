package tui

import (
	"banyan-cli/client"
	"banyan-cli/config"
	"banyan-cli/nodeproc"
	"banyan-cli/session"
	"fmt"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
)

// View mode constants
const (
	ViewStatus = iota
	ViewEvents
	ViewPeers
	ViewServices
	ViewInstances
	ViewLogs
	ViewHelp
)

// NodeInstance represents a connected or disconnected node instance
type NodeInstance struct {
	Client     *client.Client
	Address    string
	Connected  bool
	NodeProc   *nodeproc.NodeProcess
	KillOnExit bool
	Name       string // Optional friendly name
}

// Model represents the TUI state
type Model struct {
	// Configuration
	config *config.Config

	// Multi-instance connection state
	instances         []*NodeInstance
	activeInstanceIdx int
	errorCount        int // Track consecutive errors for backoff
	lastErrorTime     time.Time

	// Node state (for active instance)
	status    *client.NodeStatus
	peers     []client.PeerConnection
	events    []client.Event
	maxEvents int
	services  map[string]interface{} // Service list including beacons
	locators  map[string]interface{} // Active locators

	// Debug logs
	logs    []string
	maxLogs int

	// UI components
	cmdInput      textinput.Model
	eventView     viewport.Model
	statusView    viewport.Model
	peerView      viewport.Model
	logView       viewport.Model
	instancesView viewport.Model
	serviceView   viewport.Model
	helpView      viewport.Model

	// UI state
	currentView int
	width       int
	height      int
	err         error
	message     string
	messageTime time.Time

	// Spinner state for command execution
	executing    bool
	spinnerFrame int
	spinnerTick  time.Time

	// Title bar animation
	animFrame int

	// Command history
	cmdHistory []string
	historyIdx int
}

// NewModel creates a new TUI model
func NewModel(cfg *config.Config, nodeAddress string) Model {
	ti := textinput.New()
	ti.Placeholder = "Enter command (type 'help' for available commands)"
	ti.Focus()
	ti.CharLimit = 256
	ti.Width = 80

	m := Model{
		config:            cfg,
		instances:         make([]*NodeInstance, 0),
		activeInstanceIdx: -1,
		maxEvents:         100,
		maxLogs:           500,
		events:            make([]client.Event, 0),
		logs:              make([]string, 0),
		cmdInput:          ti,
		currentView:       ViewStatus,
		cmdHistory:        make([]string, 0),
		historyIdx:        -1,
	}

	// If initial address provided via CLI arg, prioritize it
	if nodeAddress != "" {
		m.instances = append(m.instances, &NodeInstance{
			Address:   nodeAddress,
			Connected: false,
		})
		m.activeInstanceIdx = 0
	} else if len(cfg.SavedInstances) > 0 {
		// Load saved instances from config
		for _, saved := range cfg.SavedInstances {
			m.instances = append(m.instances, &NodeInstance{
				Address:   saved.Address,
				Name:      saved.Name,
				Connected: false,
			})
		}
		// Set active index from config, but validate it
		if cfg.ActiveInstanceIndex >= 0 && cfg.ActiveInstanceIndex < len(m.instances) {
			m.activeInstanceIdx = cfg.ActiveInstanceIndex
		} else if len(m.instances) > 0 {
			m.activeInstanceIdx = 0
		}
	} else if cfg.LastNodeAddress != "" {
		// Fallback to legacy single address
		m.instances = append(m.instances, &NodeInstance{
			Address:   cfg.LastNodeAddress,
			Connected: false,
		})
		m.activeInstanceIdx = 0
	}

	return m
}

// SaveInstancesToConfig saves the current instances to the config file
func (m *Model) SaveInstancesToConfig() {
	if m.config == nil {
		return
	}
	saved := make([]config.SavedInstance, 0, len(m.instances))
	savedActiveIdx := -1
	for i, inst := range m.instances {
		// Save all instances except subprocesses (they're temporary)
		if !inst.KillOnExit {
			if i == m.activeInstanceIdx {
				savedActiveIdx = len(saved)
			}
			saved = append(saved, config.SavedInstance{
				Address: inst.Address,
				Name:    inst.Name,
			})
		}
	}
	m.config.SaveInstances(saved, savedActiveIdx)

	// Sync instance state to WebUI
	m.SyncInstancesToWebUI()
}

// SyncInstancesToWebUI broadcasts instance connection state to WebUI clients
func (m *Model) SyncInstancesToWebUI() {
	instances := make([]session.InstanceInfo, 0, len(m.instances))
	for i, inst := range m.instances {
		instances = append(instances, session.InstanceInfo{
			Index:      i + 1,
			Address:    inst.Address,
			Connected:  inst.Connected,
			Active:     i == m.activeInstanceIdx,
			Subprocess: inst.KillOnExit,
			Name:       inst.Name,
		})
	}
	session.Get().UpdateInstances(instances)

	// Also sync logged instances
	m.SyncLoggedInstancesToWebUI()
}

// SyncLoggedInstancesToWebUI broadcasts logged instances to WebUI clients
func (m *Model) SyncLoggedInstancesToWebUI() {
	if m.config == nil {
		return
	}
	loggedInstances := m.config.GetLoggedInstances()
	sessionLogged := make([]session.LoggedInstanceInfo, 0, len(loggedInstances))
	for i, logged := range loggedInstances {
		sessionLogged = append(sessionLogged, session.LoggedInstanceInfo{
			Index:     i + 1,
			Address:   logged.Address,
			Name:      logged.Name,
			LastSeen:  logged.LastSeen,
			CreatedAt: logged.CreatedAt,
		})
	}
	session.Get().UpdateLoggedInstances(sessionLogged)
}

// ActiveInstance returns the currently active instance, or nil if none
func (m Model) ActiveInstance() *NodeInstance {
	if m.activeInstanceIdx < 0 || m.activeInstanceIdx >= len(m.instances) {
		return nil
	}
	return m.instances[m.activeInstanceIdx]
}

// ActiveClient returns the client for the active instance, or nil
func (m *Model) ActiveClient() *client.Client {
	inst := m.ActiveInstance()
	if inst == nil || !inst.Connected {
		return nil
	}
	return inst.Client
}

// IsConnected returns true if there's an active connected instance
func (m Model) IsConnected() bool {
	inst := m.ActiveInstance()
	return inst != nil && inst.Connected
}

// ActiveAddress returns the address of the active instance
func (m *Model) ActiveAddress() string {
	inst := m.ActiveInstance()
	if inst == nil {
		return ""
	}
	return inst.Address
}

// AddInstance adds a new instance and returns its index
func (m *Model) AddInstance(inst *NodeInstance) int {
	m.instances = append(m.instances, inst)
	return len(m.instances) - 1
}

// AddLog adds a log entry with timestamp
func (m *Model) AddLog(message string) {
	timestamp := time.Now().Format("15:04:05")
	entry := fmt.Sprintf("[%s] %s", timestamp, message)
	m.logs = append(m.logs, entry)
	if len(m.logs) > m.maxLogs {
		m.logs = m.logs[1:]
	}
}

// Init initializes the model
func (m Model) Init() tea.Cmd {
	// Mark TUI as active so session commands can be processed
	session.Get().SetTUIActive(true)

	cmds := []tea.Cmd{textinput.Blink, listenForSessionCommands(), animTick()}

	// Connect to ALL saved instances on startup
	if len(m.instances) > 0 {
		for i, inst := range m.instances {
			if inst.Address != "" {
				cmds = append(cmds, doConnect(inst.Address, i))
			}
		}
	} else if m.config.LastNodeAddress != "" {
		// Fallback to legacy single address
		cmds = append(cmds, doConnect(m.config.LastNodeAddress, -1))
	}

	return tea.Batch(cmds...)
}

// Message types
type (
	connectMsg struct {
		address     string
		instanceIdx int // -1 means create new instance
	}

	connectedMsg struct {
		client      *client.Client
		instanceIdx int
		wsErr       error // WebSocket subscription error (nil if successful)
	}

	connectionErrorMsg struct {
		err         error
		instanceIdx int
	}

	statusUpdateMsg struct {
		status *client.NodeStatus
		err    error
	}

	peersUpdateMsg struct {
		peers []client.PeerConnection
		err   error
	}

	eventMsg struct {
		event client.Event
	}

	servicesUpdateMsg struct {
		services map[string]interface{}
		locators map[string]interface{}
		err      error
	}

	// instanceHealthMsg reports health check results for an instance
	instanceHealthMsg struct {
		instanceIdx int
		connected   bool
		err         error
	}

	nodeStartedMsg struct {
		url         string
		nodeProc    *nodeproc.NodeProcess
		killOnExit  bool
		instanceIdx int // Index of instance to update, or -1 for new
	}

	nodeStartErrorMsg struct {
		err error
	}

	commandResultMsg struct {
		result string
		err    error
	}

	tickMsg        struct{}
	healthCheckMsg struct{} // Triggers health check of all instances
	spinnerTickMsg struct{} // Spinner animation tick
	animTickMsg    struct{} // Title bar animation tick

	// instanceSelectedMsg is sent when user switches active instance
	instanceSelectedMsg struct {
		idx     int
		address string
	}

	// instanceRemovedMsg is sent when a disconnected instance is removed
	instanceRemovedMsg struct {
		idx int // 1-based index for display
	}

	// sessionCommandMsg is sent when WebUI sends a command via session
	sessionCommandMsg struct {
		cmd session.Command
	}

	// reconnectLoggedMsg is sent when user wants to reconnect to a logged instance
	reconnectLoggedMsg struct {
		address   string
		name      string
		loggedIdx int // 1-based index in logged instances list
	}

	// logMsg represents a debug log message
	logMsg struct {
		message string
	}
)

// Commands
func doConnect(address string, instanceIdx int) tea.Cmd {
	return func() tea.Msg {
		c := client.New(address)

		// Test connection with health check
		_, err := c.Health()
		if err != nil {
			return connectionErrorMsg{err: fmt.Errorf("health check failed: %w", err), instanceIdx: instanceIdx}
		}

		// Subscribe to events
		var wsErr error
		if wsErr = c.SubscribeEvents(); wsErr != nil {
			// Log the error but continue - WebSocket is not fatal
		}

		return connectedMsg{client: c, instanceIdx: instanceIdx, wsErr: wsErr}
	}
}

// listenForSessionCommands listens for commands from WebUI via session
func listenForSessionCommands() tea.Cmd {
	return func() tea.Msg {
		cmd := <-session.Get().CommandChan()
		return sessionCommandMsg{cmd: cmd}
	}
}

func fetchStatus(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		status, err := c.GetStatus()
		if err != nil {
			return statusUpdateMsg{status: nil, err: err}
		}
		return statusUpdateMsg{status: status}
	}
}

func fetchPeers(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		peers, err := c.GetConnections()
		if err != nil {
			return peersUpdateMsg{peers: nil, err: err}
		}
		return peersUpdateMsg{peers: peers}
	}
}

func fetchServices(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		services, svcErr := c.ListServices()
		locators, locErr := c.GetLocators()
		if svcErr != nil && locErr != nil {
			return servicesUpdateMsg{err: svcErr}
		}
		return servicesUpdateMsg{services: services, locators: locators}
	}
}

func listenForEvents(c *client.Client) tea.Cmd {
	return func() tea.Msg {
		if c == nil {
			return logMsg{message: "DEBUG listenForEvents: client is nil"}
		}
		select {
		case event, ok := <-c.Events():
			if !ok {
				// Channel closed
				return logMsg{message: "DEBUG listenForEvents: channel closed"}
			}
			return eventMsg{event: event}
		case <-time.After(30 * time.Second):
			// Timeout - re-trigger listening
			return logMsg{message: "DEBUG listenForEvents: 30s timeout, no events received"}
		}
	}
}

// checkInstanceHealth checks if a specific instance is still reachable
func checkInstanceHealth(inst *NodeInstance, idx int) tea.Cmd {
	return func() tea.Msg {
		if inst == nil || inst.Client == nil {
			return instanceHealthMsg{instanceIdx: idx, connected: false}
		}
		_, err := inst.Client.Health()
		return instanceHealthMsg{
			instanceIdx: idx,
			connected:   err == nil,
			err:         err,
		}
	}
}

// triggerHealthCheck triggers a health check cycle
func triggerHealthCheck() tea.Cmd {
	return func() tea.Msg {
		return healthCheckMsg{}
	}
}

func tick() tea.Cmd {
	return tea.Tick(2*time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// spinnerTick returns a command that triggers spinner animation
func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(time.Time) tea.Msg {
		return spinnerTickMsg{}
	})
}

// animTick returns a command that triggers title bar animation
func animTick() tea.Cmd {
	return tea.Tick(50*time.Millisecond, func(time.Time) tea.Msg {
		return animTickMsg{}
	})
}
