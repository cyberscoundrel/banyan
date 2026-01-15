package nodeproc

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"
)

// NodeProcess manages a Banyan node process
type NodeProcess struct {
	cmd            *exec.Cmd
	executablePath string
	configPath     string
	extraArgs      []string
	managementURL  string
	started        bool
	mu             sync.Mutex
	output         chan string
	done           chan struct{}
}

// New creates a new NodeProcess manager
func New(executablePath, configPath string, extraArgs []string) *NodeProcess {
	return &NodeProcess{
		executablePath: executablePath,
		configPath:     configPath,
		extraArgs:      extraArgs,
		output:         make(chan string, 100),
		done:           make(chan struct{}),
	}
}

// Start starts the node process
func (np *NodeProcess) Start() error {
	np.mu.Lock()
	defer np.mu.Unlock()

	if np.started {
		return fmt.Errorf("node already started")
	}

	// Check if executable exists
	if _, err := os.Stat(np.executablePath); os.IsNotExist(err) {
		return fmt.Errorf("node executable not found: %s", np.executablePath)
	}

	args := []string{}
	if np.configPath != "" {
		args = append(args, "-config", np.configPath)
	}
	// Append any extra args
	args = append(args, np.extraArgs...)

	np.cmd = exec.Command(np.executablePath, args...)

	// Capture stdout and stderr
	stdout, err := np.cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("failed to create stdout pipe: %w", err)
	}

	stderr, err := np.cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("failed to create stderr pipe: %w", err)
	}

	if err := np.cmd.Start(); err != nil {
		return fmt.Errorf("failed to start node: %w", err)
	}

	np.started = true

	// Read stdout
	go func() {
		scanner := bufio.NewScanner(stdout)
		urlRegex := regexp.MustCompile(`Management API server available at: (http://[^\s]+)`)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case np.output <- line:
			default:
			}

			// Try to extract management URL
			if matches := urlRegex.FindStringSubmatch(line); len(matches) > 1 {
				np.mu.Lock()
				np.managementURL = matches[1]
				np.mu.Unlock()
			}
		}
	}()

	// Read stderr
	go func() {
		scanner := bufio.NewScanner(stderr)
		for scanner.Scan() {
			line := scanner.Text()
			select {
			case np.output <- "[ERR] " + line:
			default:
			}
		}
	}()

	// Wait for process in background
	go func() {
		np.cmd.Wait()
		np.mu.Lock()
		np.started = false
		np.mu.Unlock()
		close(np.done)
	}()

	return nil
}

// Stop stops the node process
func (np *NodeProcess) Stop() error {
	np.mu.Lock()
	defer np.mu.Unlock()

	if !np.started || np.cmd == nil || np.cmd.Process == nil {
		return nil
	}

	// Send interrupt signal first
	if err := np.cmd.Process.Signal(os.Interrupt); err != nil {
		// If interrupt fails, kill
		np.cmd.Process.Kill()
	}

	// Wait briefly for graceful shutdown
	select {
	case <-np.done:
	case <-time.After(5 * time.Second):
		np.cmd.Process.Kill()
	}

	np.started = false
	return nil
}

// IsRunning returns whether the node is running
func (np *NodeProcess) IsRunning() bool {
	np.mu.Lock()
	defer np.mu.Unlock()
	return np.started
}

// GetManagementURL returns the management server URL if available
func (np *NodeProcess) GetManagementURL() string {
	np.mu.Lock()
	defer np.mu.Unlock()
	return np.managementURL
}

// Output returns the output channel
func (np *NodeProcess) Output() <-chan string {
	return np.output
}

// WaitForManagementURL waits for the management URL to become available
func (np *NodeProcess) WaitForManagementURL(timeout time.Duration) (string, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		url := np.GetManagementURL()
		if url != "" && strings.HasPrefix(url, "http") {
			return url, nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return "", fmt.Errorf("timeout waiting for management URL")
}
