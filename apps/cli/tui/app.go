package tui

import (
	"banyan-cli/config"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// AppModel wraps splash and main model
type AppModel struct {
	splash     SplashModel
	main       Model
	showMain   bool
	width      int
	height     int
	splashOnly bool
	skipSplash bool
}

// NewAppModel creates a new app model with splash
func NewAppModel(cfg *config.Config, nodeAddress string, showSplash bool, splashForever bool) AppModel {
	return AppModel{
		splash:     NewSplashModel(2*time.Second, splashForever),
		main:       NewModel(cfg, nodeAddress),
		splashOnly: splashForever,
		skipSplash: !showSplash,
		showMain:   !showSplash, // If skipping splash, go straight to main
	}
}

func (m AppModel) Init() tea.Cmd {
	if m.skipSplash {
		// Skip splash, initialize main directly
		return m.main.Init()
	}
	return tea.Batch(m.splash.Init())
}

func (m AppModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	// Handle window size for both models
	if wsm, ok := msg.(tea.WindowSizeMsg); ok {
		m.width = wsm.Width
		m.height = wsm.Height
	}

	if !m.showMain {
		// Update splash
		newSplash, cmd := m.splash.Update(msg)
		m.splash = newSplash
		if cmd != nil {
			cmds = append(cmds, cmd)
		}

		// Check if splash is done
		if m.splash.IsDone() {
			// In splash-only mode, quit when done
			if m.splashOnly {
				return m, tea.Quit
			}

			m.showMain = true
			// Forward window size to main model
			if m.width > 0 {
				mainModel, cmd := m.main.Update(tea.WindowSizeMsg{
					Width:  m.width,
					Height: m.height,
				})
				m.main = mainModel.(Model)
				if cmd != nil {
					cmds = append(cmds, cmd)
				}
			}
			// Initialize main model
			cmds = append(cmds, m.main.Init())
		}
	} else {
		// Update main model
		mainModel, cmd := m.main.Update(msg)
		m.main = mainModel.(Model)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m AppModel) View() string {
	if !m.showMain {
		return m.splash.View()
	}
	return m.main.View()
}
