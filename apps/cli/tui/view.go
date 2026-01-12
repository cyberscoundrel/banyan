package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

// Color scheme matching banyan logo - purples, oranges, and gold
var (
	// Primary purple from logo
	primaryPurple = lipgloss.Color("#6d3b7c")
	// Orange/amber from the tree
	accentOrange = lipgloss.Color("#cd4e34")
	// Golden/cream highlights
	accentGold = lipgloss.Color("#f8dd99")
	// Darker purple
	darkPurple = lipgloss.Color("#5a2e68")
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(accentOrange).
			Padding(0, 1)

	tabStyle = lipgloss.NewStyle().
			Padding(0, 2)

	activeTabStyle = lipgloss.NewStyle().
			Padding(0, 2).
			Background(primaryPurple).
			Foreground(accentGold)

	statusBarStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("241"))

	errorStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#ff6b6b"))

	successStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#69db7c"))

	infoStyle = lipgloss.NewStyle().
			Foreground(accentGold)

	borderStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(primaryPurple).
			Padding(1)
)

// View renders the UI
func (m Model) View() string {
	var b strings.Builder

	// Animated title bar
	b.WriteString(m.renderAnimatedTitle())
	b.WriteString("\n")

	// Connection status - only ONE status line
	if m.IsConnected() {
		b.WriteString(successStyle.Render(fmt.Sprintf("● Connected to %s [%d/%d]", m.ActiveAddress(), m.activeInstanceIdx+1, len(m.instances))))
	} else if len(m.instances) > 0 {
		b.WriteString(errorStyle.Render(fmt.Sprintf("○ Disconnected [%d instances]", len(m.instances))))
	} else {
		b.WriteString(errorStyle.Render("○ No instances"))
	}
	b.WriteString("\n")

	// Tab bar (Ctrl+N to switch) - use short names for narrow windows
	var tabs []string
	if m.width < 70 {
		tabs = []string{"Sts", "Evt", "Prs", "Svc", "Ins", "Log", "?"}
	} else {
		tabs = []string{"Status", "Events", "Peers", "Services", "Instances", "Logs", "Help"}
	}
	var tabBar strings.Builder
	for i, tab := range tabs {
		if i == m.currentView {
			tabBar.WriteString(activeTabStyle.Render(tab))
		} else {
			tabBar.WriteString(tabStyle.Render(tab))
		}
	}
	b.WriteString(tabBar.String())
	b.WriteString("\n")
	// Separator line - clipped to width
	sepWidth := m.width
	if sepWidth < 1 {
		sepWidth = 80
	}
	b.WriteString(strings.Repeat("─", sepWidth))
	b.WriteString("\n")

	// Content area
	switch m.currentView {
	case ViewStatus:
		b.WriteString(m.statusView.View())
	case ViewEvents:
		b.WriteString(m.eventView.View())
	case ViewPeers:
		b.WriteString(m.peerView.View())
	case ViewServices:
		b.WriteString(m.serviceView.View())
	case ViewInstances:
		b.WriteString(m.instancesView.View())
	case ViewLogs:
		b.WriteString(m.logView.View())
	case ViewHelp:
		b.WriteString(m.helpView.View())
	}

	b.WriteString("\n")
	b.WriteString(strings.Repeat("─", sepWidth))
	b.WriteString("\n")

	// Spinner or message area
	if m.executing {
		// Show spinner while executing
		spinnerFrames := []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}
		frame := spinnerFrames[m.spinnerFrame%len(spinnerFrames)]
		b.WriteString(infoStyle.Render(frame + " Executing..."))
		b.WriteString("\n")
	} else if m.message != "" && time.Since(m.messageTime) < 10*time.Second {
		// Show message after command completes
		msg := m.message
		maxLen := m.width - 6 // Account for icon and padding
		if maxLen < 20 {
			maxLen = 20
		}
		if len(msg) > maxLen {
			msg = msg[:maxLen-3] + "..."
		}
		if m.err != nil {
			b.WriteString(errorStyle.Render("❌ " + msg))
		} else {
			b.WriteString(infoStyle.Render("ℹ️  " + msg))
		}
		b.WriteString("\n")
	}

	// Command input
	b.WriteString("\n")
	b.WriteString("> ")
	b.WriteString(m.cmdInput.View())
	b.WriteString("\n")

	// Footer - use short text for narrow windows
	var footer string
	if m.width < 60 {
		footer = "Tab:views • help • Ctrl+Q"
	} else {
		footer = "Tab/Ctrl+N to switch views • Type 'help' for commands • Ctrl+Q to quit"
	}
	b.WriteString(statusBarStyle.Render(footer))

	return b.String()
}

func (m Model) renderStatusContent() string {
	if m.status == nil {
		return "No status data available. Connect to a node first."
	}

	var b strings.Builder
	s := m.status

	b.WriteString(fmt.Sprintf("Node ID: %s\n", s.NodeID))
	b.WriteString(fmt.Sprintf("DHT Enabled: %v\n", s.DHTEnabled))
	b.WriteString(fmt.Sprintf("Discovery Methods: %v\n", s.DiscoveryMethods))
	b.WriteString("\n")
	b.WriteString("Peer Statistics:\n")
	b.WriteString(fmt.Sprintf("  Total Connected: %d\n", s.TotalConnectedPeers))
	b.WriteString(fmt.Sprintf("  Tracked Peers: %d\n", s.TrackedPeers))
	b.WriteString(fmt.Sprintf("  HTTP Capable: %d\n", s.HTTPCapablePeers))
	b.WriteString(fmt.Sprintf("  Bidirectional HTTP: %d\n", s.BidirectionalPeers))
	b.WriteString("\n")
	b.WriteString("Connection Types:\n")
	for connType, count := range s.ConnectionTypes {
		b.WriteString(fmt.Sprintf("  %s: %d\n", connType, count))
	}
	b.WriteString("\n")
	b.WriteString("Listening Addresses:\n")
	for _, addr := range s.ListeningAddrs {
		b.WriteString(fmt.Sprintf("  %v\n", addr))
	}
	b.WriteString(fmt.Sprintf("\nLast Updated: %s\n", s.Timestamp.Format(time.RFC3339)))

	return b.String()
}

func (m Model) renderEventsContent() string {
	if len(m.events) == 0 {
		return "No events yet. Events will appear here as they occur."
	}

	var b strings.Builder
	for i := len(m.events) - 1; i >= 0; i-- {
		e := m.events[i]
		timestamp := e.Timestamp.Format("15:04:05")
		b.WriteString(fmt.Sprintf("[%s] %s: %v\n", timestamp, e.Type, e.Data))
	}
	return b.String()
}

func (m Model) renderPeersContent() string {
	if len(m.peers) == 0 {
		return "No peers connected."
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf("Total Peers: %d\n\n", len(m.peers)))

	for _, p := range m.peers {
		statusIcon := "○"
		if p.Connected {
			statusIcon = "●"
		}
		b.WriteString(fmt.Sprintf("%s [%s] %s\n", statusIcon, p.Alias, truncate(p.PeerID, 20)))
		b.WriteString(fmt.Sprintf("    Type: %s | HTTP: %v | Status: %s\n", p.ConnectionType, p.HTTPCapable, p.Status))
	}
	return b.String()
}

// renderAnimatedTitle renders an animated title bar with banyan color scheme
func (m Model) renderAnimatedTitle() string {
	// Smooth banyan gradient - many intermediate colors for fluid animation
	colors := []lipgloss.Color{
		// Dark purple to primary purple
		lipgloss.Color("#5a2e68"),
		lipgloss.Color("#5e3170"),
		lipgloss.Color("#623475"),
		lipgloss.Color("#66377a"),
		lipgloss.Color("#6a3a7e"),
		lipgloss.Color("#6d3b7c"),
		// Primary purple to purple-pink
		lipgloss.Color("#703d79"),
		lipgloss.Color("#743f76"),
		lipgloss.Color("#784173"),
		lipgloss.Color("#7c4370"),
		lipgloss.Color("#80456d"),
		lipgloss.Color("#84476a"),
		lipgloss.Color("#884967"),
		lipgloss.Color("#8b4a6b"),
		// Purple-pink to mauve
		lipgloss.Color("#8f4c67"),
		lipgloss.Color("#934e63"),
		lipgloss.Color("#97505f"),
		lipgloss.Color("#9b525b"),
		lipgloss.Color("#9f5457"),
		lipgloss.Color("#a35653"),
		lipgloss.Color("#a7584f"),
		lipgloss.Color("#a85a5a"),
		// Mauve to orange
		lipgloss.Color("#af5753"),
		lipgloss.Color("#b6544c"),
		lipgloss.Color("#bd5145"),
		lipgloss.Color("#c44e3e"),
		lipgloss.Color("#c84f38"),
		lipgloss.Color("#cd4e34"),
		// Orange back to mauve
		lipgloss.Color("#c84f38"),
		lipgloss.Color("#c44e3e"),
		lipgloss.Color("#bd5145"),
		lipgloss.Color("#b6544c"),
		lipgloss.Color("#af5753"),
		lipgloss.Color("#a85a5a"),
		// Mauve back to purple-pink
		lipgloss.Color("#a35653"),
		lipgloss.Color("#9f5457"),
		lipgloss.Color("#9b525b"),
		lipgloss.Color("#97505f"),
		lipgloss.Color("#934e63"),
		lipgloss.Color("#8f4c67"),
		lipgloss.Color("#8b4a6b"),
		// Purple-pink back to primary purple
		lipgloss.Color("#884967"),
		lipgloss.Color("#84476a"),
		lipgloss.Color("#80456d"),
		lipgloss.Color("#7c4370"),
		lipgloss.Color("#784173"),
		lipgloss.Color("#743f76"),
		lipgloss.Color("#703d79"),
		lipgloss.Color("#6d3b7c"),
		// Primary purple back to dark purple
		lipgloss.Color("#6a3a7e"),
		lipgloss.Color("#66377a"),
		lipgloss.Color("#623475"),
		lipgloss.Color("#5e3170"),
		lipgloss.Color("#5a2e68"),
	}

	// Full terminal width
	width := m.width
	if width < 20 {
		width = 80
	}

	// Build the animated bar
	var bar strings.Builder
	title := "BANYAN"
	titleLen := len(title)

	// Slow gradient movement - divide frame by 8 for very slow drift
	animOffset := m.animFrame / 8

	for i := 0; i < width; i++ {
		// Stretch the gradient across the width, with slow animation
		colorIdx := ((i * len(colors) / width) + animOffset) % len(colors)
		bgColor := colors[colorIdx]

		// Check if this position is part of the title (with 1 char padding)
		if i >= 1 && i < titleLen+1 {
			// Title character - black text on colored background
			char := string(title[i-1])
			style := lipgloss.NewStyle().
				Background(bgColor).
				Foreground(lipgloss.Color("#000000")).
				Bold(true)
			bar.WriteString(style.Render(char))
		} else {
			// Background bar - just the colored block
			style := lipgloss.NewStyle().Background(bgColor)
			bar.WriteString(style.Render(" "))
		}
	}

	return bar.String()
}
