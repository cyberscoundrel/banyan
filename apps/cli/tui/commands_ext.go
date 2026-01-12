package tui

import (
	"banyan-cli/client"
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

func (m Model) cmdServices() tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}
	// Fetch services and locators to update the Services view
	return fetchServices(c)
}

func (m Model) cmdFigs() tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}

	return func() tea.Msg {
		figs, err := c.GetServiceFigs()
		if err != nil {
			return commandResultMsg{err: err}
		}
		var sb strings.Builder
		sb.WriteString("Service Figs:\n")
		for _, fig := range figs {
			sb.WriteString(fmt.Sprintf("  - %s (key: %s...)\n", fig.ServiceAlias, truncate(fig.ServiceKey, 16)))
		}
		return commandResultMsg{result: sb.String()}
	}
}

func (m Model) cmdServe(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}

	if len(args) == 0 {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: serve <start|stop> [args]")} }
	}

	action := strings.ToLower(args[0])
	switch action {
	case "start":
		if len(args) < 2 {
			return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: serve start <file_location>")} }
		}
		return func() tea.Msg {
			req := client.ServiceBeaconRequest{FileLocation: args[1]}
			resp, err := c.StartServiceBeacon(req)
			if err != nil {
				return commandResultMsg{err: err}
			}
			return commandResultMsg{result: fmt.Sprintf("Service beacon started: %v", resp)}
		}
	case "stop":
		hash := ""
		if len(args) > 1 {
			hash = args[1]
		}
		return func() tea.Msg {
			err := c.StopServiceBeacon(hash)
			if err != nil {
				return commandResultMsg{err: err}
			}
			return commandResultMsg{result: "Service beacon stopped"}
		}
	default:
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("unknown serve action: %s", action)} }
	}
}

func (m Model) cmdProxy(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}

	if len(args) < 3 {
		return func() tea.Msg {
			return commandResultMsg{err: fmt.Errorf("usage: proxy <peer|service> <id> <path> [method]")}
		}
	}

	proxyType := strings.ToLower(args[0])
	id := args[1]
	path := args[2]
	method := "GET"
	if len(args) > 3 {
		method = strings.ToUpper(args[3])
	}

	return func() tea.Msg {
		var body []byte
		var statusCode int
		var err error

		switch proxyType {
		case "peer":
			body, statusCode, err = c.ProxyPeerRequest(id, path, method, nil)
		case "service":
			body, statusCode, err = c.ProxyServiceRequest(id, path, method, nil)
		default:
			return commandResultMsg{err: fmt.Errorf("unknown proxy type: %s (use 'peer' or 'service')", proxyType)}
		}

		if err != nil {
			return commandResultMsg{err: err}
		}

		result := fmt.Sprintf("[%d] %s", statusCode, truncate(string(body), 500))
		return commandResultMsg{result: result}
	}
}

func (m Model) cmdRoute(args []string) tea.Cmd {
	c := m.ActiveClient()
	if c == nil {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("not connected")} }
	}

	if len(args) == 0 {
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: route <list|add> [args]")} }
	}

	action := strings.ToLower(args[0])
	switch action {
	case "list":
		return func() tea.Msg {
			routes, err := c.GetRoutes()
			if err != nil {
				return commandResultMsg{err: err}
			}
			var sb strings.Builder
			sb.WriteString("Routes:\n")
			for _, r := range routes {
				sb.WriteString(fmt.Sprintf("  %s -> %s (%v)\n", r.Path, r.Target, r.Methods))
			}
			return commandResultMsg{result: sb.String()}
		}
	case "add":
		if len(args) < 3 {
			return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("usage: route add <path> <target>")} }
		}
		return func() tea.Msg {
			req := client.RouteAddRequest{
				Path:   args[1],
				Target: args[2],
			}
			err := c.AddRoute(req)
			if err != nil {
				return commandResultMsg{err: err}
			}
			return commandResultMsg{result: "Route added"}
		}
	default:
		return func() tea.Msg { return commandResultMsg{err: fmt.Errorf("unknown route action: %s", action)} }
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
