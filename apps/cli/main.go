package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"banyan-cli/client"
	"banyan-cli/config"
	"banyan-cli/server"
	"banyan-cli/tui"
)

var (
	configFile    = flag.String("config", "", "Path to config file (default: banyan-cli.json in exe dir)")
	nodeAddress   = flag.String("node", "", "Address of node to connect to (e.g., http://localhost:8080)")
	webOnly       = flag.Bool("web", false, "Run web UI only (no TUI)")
	webPort       = flag.Int("port", 0, "Port for web UI (default: from config or 8080)")
	noWeb         = flag.Bool("no-web", false, "Disable web UI")
	version       = flag.Bool("version", false, "Print version and exit")
	splashForever = flag.Bool("splash", false, "Show splash screen indefinitely")
	noSplash      = flag.Bool("nosplash", false, "Skip splash screen entirely")
)

const Version = "0.1.0"

func main() {
	flag.Parse()

	if *version {
		fmt.Printf("Banyan CLI v%s\n", Version)
		os.Exit(0)
	}

	// Load configuration
	var cfg *config.Config
	if *configFile != "" {
		var err error
		cfg, err = config.LoadFrom(*configFile)
		if err != nil {
			log.Fatalf("Failed to load config from %s: %v", *configFile, err)
		}
	} else {
		cfg = config.LoadOrDefault()
	}

	// Override config with CLI flags (flags take precedence)
	if *webPort > 0 {
		cfg.WebPort = *webPort
	}
	if cfg.WebPort == 0 {
		cfg.WebPort = 8080
	}
	if *noWeb {
		cfg.NoWeb = true
	}
	if *noSplash {
		cfg.NoSplash = true
	}

	// Determine node address
	address := *nodeAddress
	if address == "" && cfg.DefaultNodeAddress != "" {
		address = cfg.DefaultNodeAddress
	}

	// Start web server if not disabled
	var webServer *server.Server
	if !cfg.NoWeb {
		webServer = server.New(cfg, cfg.WebPort)

		// If we have an address, connect to it
		if address != "" {
			c := client.New(address)
			if _, err := c.Health(); err == nil {
				c.SubscribeEvents()
				webServer.SetClient(c)
			}
		}

		if err := webServer.Start(); err != nil {
			log.Printf("Failed to start web server: %v", err)
		} else {
			fmt.Printf("Web UI available at http://localhost:%d\n", cfg.WebPort)
		}
	}

	// If web-only mode, just wait
	if *webOnly {
		fmt.Println("Running in web-only mode. Press Ctrl+C to exit.")
		select {} // Block forever
	}

	// Determine splash mode: nosplash takes precedence over splash
	showSplash := !cfg.NoSplash
	splashPermanent := *splashForever && !cfg.NoSplash

	// Start TUI with splash screen
	model := tui.NewAppModel(cfg, address, showSplash, splashPermanent)

	p := tea.NewProgram(model, tea.WithAltScreen())

	if _, err := p.Run(); err != nil {
		log.Fatal(err)
	}

	// Cleanup
	if webServer != nil {
		webServer.Stop()
	}
}
