package tui

import (
	_ "embed"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

//go:embed banyan.txt
var banyanLogo string

//go:embed banyansm.txt
var banyanLogoSmall string

// Color scheme matching banyan.svg - purples, oranges, and cream
var (
	// Primary purple from logo
	colorPurple = lipgloss.Color("#6d3b7c")
	// Orange/amber from the tree trunk
	colorOrange = lipgloss.Color("#cd4e34")
	// Golden/cream from highlights
	colorGold = lipgloss.Color("#f8dd99")
	// Darker purple for accents
	colorDarkPurple = lipgloss.Color("#5a2e68")
)

// Splash screen styles
var (
	splashOrange = lipgloss.NewStyle().Foreground(colorOrange)
	splashGold   = lipgloss.NewStyle().Foreground(colorGold)
)

const banyanBlockText = `
██████╗  █████╗ ███╗   ██╗██╗   ██╗ █████╗ ███╗   ██╗
██╔══██╗██╔══██╗████╗  ██║╚██╗ ██╔╝██╔══██╗████╗  ██║
██████╔╝███████║██╔██╗ ██║ ╚████╔╝ ███████║██╔██╗ ██║
██╔══██╗██╔══██║██║╚██╗██║  ╚██╔╝  ██╔══██║██║╚██╗██║
██████╔╝██║  ██║██║ ╚████║   ██║   ██║  ██║██║ ╚████║
╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═══╝   ╚═╝   ╚═╝  ╚═╝╚═╝  ╚═══╝
`

const banyanBlockTextSmall = `
█▄▄ ▄▀█ █▄ █ █▄█ ▄▀█ █▄ █
█▄█ █▀█ █ ▀█  █  █▀█ █ ▀█
`

// SplashModel is a model for the splash screen
type SplashModel struct {
	width    int
	height   int
	duration time.Duration
	done     bool
	forever  bool
}

// NewSplashModel creates a new splash model
func NewSplashModel(duration time.Duration, forever bool) SplashModel {
	return SplashModel{
		duration: duration,
		forever:  forever,
	}
}

type splashDoneMsg struct{}

func (m SplashModel) Init() tea.Cmd {
	if m.forever {
		return nil // No timeout
	}
	return tea.Tick(m.duration, func(t time.Time) tea.Msg {
		return splashDoneMsg{}
	})
}

func (m SplashModel) Update(msg tea.Msg) (SplashModel, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
	case splashDoneMsg:
		if !m.forever {
			m.done = true
		}
	case tea.KeyMsg:
		// In forever mode, only 'q' or ctrl+q quits
		if m.forever {
			if msg.String() == "q" || msg.String() == "ctrl+q" {
				m.done = true
			}
		} else {
			// Normal splash: any key skips
			m.done = true
		}
	}
	return m, nil
}

func (m SplashModel) IsDone() bool {
	return m.done
}

func (m SplashModel) View() string {
	if m.width == 0 {
		return ""
	}

	// Use small versions for narrow terminals (under 80 columns)
	useSmall := m.width < 80

	// Select the appropriate logo
	var logo string
	if useSmall {
		logo = strings.TrimRight(banyanLogoSmall, "\n\r")
	} else {
		logo = strings.TrimRight(banyanLogo, "\n\r")
	}
	// Clean up carriage returns
	logo = strings.ReplaceAll(logo, "\r", "")

	// Select the appropriate block text
	var blockTextSrc string
	if useSmall {
		blockTextSrc = banyanBlockTextSmall
	} else {
		blockTextSrc = banyanBlockText
	}

	// Color the block text
	blockLines := strings.Split(strings.TrimPrefix(blockTextSrc, "\n"), "\n")
	var coloredBlock []string
	for _, line := range blockLines {
		if line != "" {
			coloredBlock = append(coloredBlock, splashOrange.Render(line))
		}
	}

	// Overlay block text on bottom of logo
	logoLines := strings.Split(logo, "\n")
	numLogoLines := len(logoLines)
	numBlockLines := len(coloredBlock)

	// Replace the last N lines of the logo with block text (centered)
	// Leave a gap at bottom of logo
	startOverlay := numLogoLines - numBlockLines - 4
	if startOverlay < 0 {
		startOverlay = 0
	}

	for i, blockLine := range coloredBlock {
		targetLine := startOverlay + i
		if targetLine < numLogoLines {
			logoLines[targetLine] = blockLine
		}
	}

	content := strings.Join(logoLines, "\n")

	// Add subtitle and hints below
	subtitle := splashGold.Render("CLI Dashboard")
	version := lipgloss.NewStyle().Foreground(lipgloss.Color("#888")).Render("v0.1.0")

	var skipHint string
	if m.forever {
		skipHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666")).Render("Press 'q' to quit")
	} else {
		skipHint = lipgloss.NewStyle().Foreground(lipgloss.Color("#666")).Render("Press any key to continue...")
	}

	content = content + "\n\n" + subtitle + " " + version + "\n\n" + skipHint

	// Center the content
	style := lipgloss.NewStyle().
		Width(m.width).
		Height(m.height).
		Align(lipgloss.Center, lipgloss.Center)

	return style.Render(content)
}
