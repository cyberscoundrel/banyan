package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

const ConfigFileName = "banyan-cli.json"

// SavedInstance represents a saved node instance
type SavedInstance struct {
	Address string `json:"address"`
	Name    string `json:"name,omitempty"`
}

// LoggedInstance represents a logged node instance (not auto-reconnected)
type LoggedInstance struct {
	Address   string `json:"address"`
	Name      string `json:"name,omitempty"`
	LastSeen  string `json:"lastSeen,omitempty"`  // ISO timestamp of last connection
	CreatedAt string `json:"createdAt,omitempty"` // ISO timestamp when first logged
}

// Config holds the CLI configuration
type Config struct {
	// NodeExecutable is the path to the node executable (for 'start' command)
	NodeExecutable string `json:"nodeExecutable,omitempty"`
	// NodeConfigPath is the optional path to the node's config file
	NodeConfigPath string `json:"nodeConfigPath,omitempty"`
	// DefaultNodeAddress is the default address to connect to if no argument provided
	DefaultNodeAddress string `json:"defaultNodeAddress,omitempty"`
	// WebPort is the port for the web UI server
	WebPort int `json:"webPort,omitempty"`
	// NoWeb disables the web UI
	NoWeb bool `json:"noWeb,omitempty"`
	// NoSplash skips the splash screen
	NoSplash bool `json:"noSplash,omitempty"`
	// LastNodeAddress is the last node address the CLI connected to (for auto-reconnect) - DEPRECATED
	LastNodeAddress string `json:"lastNodeAddress,omitempty"`
	// SavedInstances is the list of node instances to reconnect to on startup
	SavedInstances []SavedInstance `json:"savedInstances,omitempty"`
	// ActiveInstanceIndex is the index of the last active instance
	ActiveInstanceIndex int `json:"activeInstanceIndex,omitempty"`
	// LoggedInstances is a historical log of all node instances ever connected to (not auto-reconnected)
	LoggedInstances []LoggedInstance `json:"loggedInstances,omitempty"`
	// configPath stores where this config was loaded from (not serialized)
	configPath string `json:"-"`
}

// GetConfigPath returns the path this config was loaded from
func (c *Config) GetConfigPath() string {
	return c.configPath
}

// SetLastNodeAddress updates the last node address and saves to config file (legacy, for backwards compat)
func (c *Config) SetLastNodeAddress(address string) error {
	c.LastNodeAddress = address
	// Also add to saved instances if not already there
	c.AddSavedInstance(address, "")
	if c.configPath != "" {
		return c.Save(c.configPath)
	}
	return nil
}

// AddSavedInstance adds an instance to the saved list if not already present
func (c *Config) AddSavedInstance(address, name string) {
	// Check if already exists
	for i, inst := range c.SavedInstances {
		if inst.Address == address {
			// Update name if provided
			if name != "" && inst.Name != name {
				c.SavedInstances[i].Name = name
			}
			return
		}
	}
	c.SavedInstances = append(c.SavedInstances, SavedInstance{Address: address, Name: name})
}

// RemoveSavedInstance removes an instance from the saved list
func (c *Config) RemoveSavedInstance(address string) {
	for i, inst := range c.SavedInstances {
		if inst.Address == address {
			c.SavedInstances = append(c.SavedInstances[:i], c.SavedInstances[i+1:]...)
			return
		}
	}
}

// SetActiveInstanceIndex sets the active instance and saves
func (c *Config) SetActiveInstanceIndex(index int) error {
	c.ActiveInstanceIndex = index
	if c.configPath != "" {
		return c.Save(c.configPath)
	}
	return nil
}

// SaveInstances saves the current instances configuration
func (c *Config) SaveInstances(instances []SavedInstance, activeIndex int) error {
	c.SavedInstances = instances
	c.ActiveInstanceIndex = activeIndex
	if c.configPath != "" {
		return c.Save(c.configPath)
	}
	return nil
}

// AddLoggedInstance adds an instance to the logged instances list if not already present
// It automatically persists the change to the configuration file
func (c *Config) AddLoggedInstance(address, name string) {
	now := time.Now().Format(time.RFC3339)
	// Check if already exists
	for i, inst := range c.LoggedInstances {
		if inst.Address == address {
			// Update last seen time and name if provided
			c.LoggedInstances[i].LastSeen = now
			if name != "" && inst.Name != name {
				c.LoggedInstances[i].Name = name
			}
			// Auto-save after modification
			c.SaveLoggedInstances()
			return
		}
	}
	// Add new logged instance
	c.LoggedInstances = append(c.LoggedInstances, LoggedInstance{
		Address:   address,
		Name:      name,
		CreatedAt: now,
		LastSeen:  now,
	})
	// Auto-save after modification
	c.SaveLoggedInstances()
}

// RemoveLoggedInstance removes an instance from the logged instances list by index (1-based)
// It automatically persists the change to the configuration file
func (c *Config) RemoveLoggedInstance(index int) bool {
	idx := index - 1 // Convert to 0-based
	if idx < 0 || idx >= len(c.LoggedInstances) {
		return false
	}
	c.LoggedInstances = append(c.LoggedInstances[:idx], c.LoggedInstances[idx+1:]...)
	// Auto-save after modification
	c.SaveLoggedInstances()
	return true
}

// ClearLoggedInstances removes all logged instances
// It automatically persists the change to the configuration file
func (c *Config) ClearLoggedInstances() {
	c.LoggedInstances = nil
	// Auto-save after modification
	c.SaveLoggedInstances()
}

// GetLoggedInstances returns a copy of the logged instances list
func (c *Config) GetLoggedInstances() []LoggedInstance {
	result := make([]LoggedInstance, len(c.LoggedInstances))
	copy(result, c.LoggedInstances)
	return result
}

// SaveLoggedInstances saves the logged instances to config
func (c *Config) SaveLoggedInstances() error {
	if c.configPath != "" {
		return c.Save(c.configPath)
	}
	return nil
}

// DefaultConfig returns a config with default values
func DefaultConfig() *Config {
	return &Config{
		WebPort: 8080,
	}
}

// LoadFrom loads configuration from a specific file path
func LoadFrom(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	config := DefaultConfig()
	if err := json.Unmarshal(data, config); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	// Store the config path for later saving
	absPath, _ := filepath.Abs(path)
	config.configPath = absPath

	// Make relative paths absolute relative to config file location
	configDir := filepath.Dir(path)
	if config.NodeExecutable != "" && !filepath.IsAbs(config.NodeExecutable) {
		config.NodeExecutable = filepath.Join(configDir, config.NodeExecutable)
	}
	if config.NodeConfigPath != "" && !filepath.IsAbs(config.NodeConfigPath) {
		config.NodeConfigPath = filepath.Join(configDir, config.NodeConfigPath)
	}

	return config, nil
}

// Load loads configuration from the config file in the executable's directory
func Load() (*Config, error) {
	// Get executable directory
	exePath, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("failed to get executable path: %w", err)
	}
	exeDir := filepath.Dir(exePath)

	// Try executable directory first, then current directory
	configPaths := []string{
		filepath.Join(exeDir, ConfigFileName),
		filepath.Join(".", ConfigFileName),
	}

	for _, p := range configPaths {
		if _, err := os.Stat(p); err == nil {
			return LoadFrom(p)
		}
	}

	return nil, fmt.Errorf("config file %s not found in executable directory or current directory", ConfigFileName)
}

// LoadOrDefault loads configuration or returns default if not found
func LoadOrDefault() *Config {
	config, err := Load()
	if err != nil {
		return DefaultConfig()
	}
	return config
}

// Save saves the configuration to the specified path
func (c *Config) Save(path string) error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0644); err != nil {
		return fmt.Errorf("failed to write config file: %w", err)
	}
	return nil
}
