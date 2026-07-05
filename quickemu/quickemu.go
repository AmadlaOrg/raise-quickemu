package quickemu

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

var ExecCommand = exec.Command

// VMConfig holds the configuration for a quickemu VM.
type VMConfig struct {
	Name          string `json:"name" yaml:"name"`
	OS            string `json:"os" yaml:"os"`
	Release       string `json:"release" yaml:"release"`
	Edition       string `json:"edition" yaml:"edition"`
	CPUs          int    `json:"cpus" yaml:"cpus"`
	Memory        string `json:"memory" yaml:"memory"`
	DiskSize      string `json:"disk_size" yaml:"disk_size"`
	Display       string `json:"display" yaml:"display"`
	SSHPort       int    `json:"ssh_port" yaml:"ssh_port"`
	SSHUser       string `json:"ssh_user" yaml:"ssh_user"`
	SSHPrivateKey string `json:"ssh_private_key" yaml:"ssh_private_key"`
}

// VMState represents the current state of a managed VM.
type VMState struct {
	Name      string `json:"name"`
	OS        string `json:"os"`
	Release   string `json:"release"`
	State     string `json:"state"`
	Provider  string `json:"provider"`
	SSHPort   int    `json:"ssh_port,omitempty"`
	ConfFile  string `json:"conf_file"`
	CreatedAt string `json:"created_at"`
}

// Manager defines the interface for managing quickemu VMs.
type Manager interface {
	Up(config *VMConfig) (*VMState, error)
	Halt(name string) error
	Destroy(name string) error
	SSH(name string, user string, port int, keyFile string) error
	Status(name string) (*VMState, error)
	StatusAll() ([]VMState, error)
}

type manager struct {
	stateManager StateManager
	workDir      string
}

// New creates a new quickemu manager.
func New() Manager {
	return &manager{
		stateManager: NewStateManager(),
		workDir:      defaultWorkDir(),
	}
}

// NewWithState creates a new quickemu manager with a custom state manager (for testing).
func NewWithState(sm StateManager, workDir string) Manager {
	return &manager{
		stateManager: sm,
		workDir:      workDir,
	}
}

func defaultWorkDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".local", "share", "raise", "quickemu", "vms")
}

// Up creates and starts a VM using quickget + quickemu.
func (m *manager) Up(config *VMConfig) (*VMState, error) {
	if config.OS == "" {
		return nil, fmt.Errorf("OS is required")
	}
	if config.Release == "" {
		return nil, fmt.Errorf("release is required")
	}

	// Set defaults
	if config.Name == "" {
		config.Name = fmt.Sprintf("%s-%s", config.OS, config.Release)
		if config.Edition != "" {
			config.Name += "-" + config.Edition
		}
	}
	if config.Memory == "" {
		config.Memory = "4G"
	}
	if config.DiskSize == "" {
		config.DiskSize = "32G"
	}
	if config.Display == "" {
		config.Display = "none"
	}
	if config.SSHUser == "" {
		config.SSHUser = "user"
	}
	if config.SSHPort <= 0 {
		config.SSHPort = 22220
	}

	// Ensure work directory exists
	if err := os.MkdirAll(m.workDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create work directory: %w", err)
	}

	// Step 1: Run quickget to download image and create .conf
	quickgetArgs := []string{config.OS, config.Release}
	if config.Edition != "" {
		quickgetArgs = append(quickgetArgs, config.Edition)
	}

	cmd := ExecCommand("quickget", quickgetArgs...)
	cmd.Dir = m.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("quickget failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	// Determine conf file name
	confFile := m.confFilePath(config)

	// Step 2: Customize .conf if needed
	if err := m.customizeConf(confFile, config); err != nil {
		return nil, fmt.Errorf("failed to customize config: %w", err)
	}

	// Step 3: Launch VM with quickemu
	launchArgs := []string{"--vm", confFile, "--display", config.Display}
	cmd = ExecCommand("quickemu", launchArgs...)
	cmd.Dir = m.workDir
	output, err = cmd.CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("quickemu failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	state := &VMState{
		Name:      config.Name,
		OS:        config.OS,
		Release:   config.Release,
		State:     "running",
		Provider:  "quickemu",
		SSHPort:   config.SSHPort,
		ConfFile:  confFile,
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}

	if err := m.stateManager.Add(*state); err != nil {
		return nil, fmt.Errorf("VM created but failed to save state: %w", err)
	}

	return state, nil
}

// Halt stops a running VM.
func (m *manager) Halt(name string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("VM %q not found in state: %w", name, err)
	}

	cmd := ExecCommand("quickemu", "--vm", stored.ConfFile, "--kill")
	cmd.Dir = m.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("quickemu --kill failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	if err := m.stateManager.UpdateState(name, "stopped"); err != nil {
		return fmt.Errorf("VM stopped but failed to update state: %w", err)
	}

	return nil
}

// Destroy removes a VM and all its files.
func (m *manager) Destroy(name string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("VM %q not found in state: %w", name, err)
	}

	// Kill if running
	cmd := ExecCommand("quickemu", "--vm", stored.ConfFile, "--kill")
	cmd.Dir = m.workDir
	_ = cmd.Run()

	// Delete VM files
	cmd = ExecCommand("quickemu", "--vm", stored.ConfFile, "--delete-vm")
	cmd.Dir = m.workDir
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("quickemu --delete-vm failed: %s: %w", strings.TrimSpace(string(output)), err)
	}

	if err := m.stateManager.Remove(name); err != nil {
		return fmt.Errorf("VM destroyed but failed to update state: %w", err)
	}

	return nil
}

// SSH connects to a VM via SSH, replacing the current process.
func (m *manager) SSH(name string, user string, port int, keyFile string) error {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return fmt.Errorf("VM %q not found in state: %w", name, err)
	}

	if user == "" {
		user = "user"
	}
	if port <= 0 {
		port = stored.SSHPort
	}
	if port <= 0 {
		port = 22220
	}

	sshArgs := []string{
		"-o", "StrictHostKeyChecking=no",
		"-o", "UserKnownHostsFile=" + os.DevNull,
		"-p", fmt.Sprintf("%d", port),
	}

	if keyFile != "" {
		sshArgs = append(sshArgs, "-i", keyFile)
	}

	sshArgs = append(sshArgs, fmt.Sprintf("%s@localhost", user))

	cmd := ExecCommand("ssh", sshArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// Status returns the current state of a specific VM.
func (m *manager) Status(name string) (*VMState, error) {
	stored, err := m.stateManager.Get(name)
	if err != nil {
		return nil, fmt.Errorf("VM %q not found in state: %w", name, err)
	}

	// Check if VM is actually running by looking for PID file
	running := m.isRunning(stored.ConfFile)
	if running {
		stored.State = "running"
	} else {
		stored.State = "stopped"
	}

	_ = m.stateManager.UpdateState(name, stored.State)

	return stored, nil
}

// StatusAll returns the status of all managed VMs.
func (m *manager) StatusAll() ([]VMState, error) {
	states, err := m.stateManager.LoadAll()
	if err != nil {
		return nil, fmt.Errorf("failed to load state: %w", err)
	}

	if len(states) == 0 {
		return []VMState{}, nil
	}

	// Refresh running state
	for i := range states {
		if m.isRunning(states[i].ConfFile) {
			states[i].State = "running"
		} else {
			states[i].State = "stopped"
		}
	}

	return states, nil
}

// confFilePath determines the .conf file path for a given config.
func (m *manager) confFilePath(config *VMConfig) string {
	name := fmt.Sprintf("%s-%s", config.OS, config.Release)
	if config.Edition != "" {
		name += "-" + config.Edition
	}
	return filepath.Join(m.workDir, name+".conf")
}

// customizeConf appends custom settings to the .conf file.
func (m *manager) customizeConf(confFile string, config *VMConfig) error {
	var additions []string

	if config.CPUs > 0 {
		additions = append(additions, fmt.Sprintf("cpu_cores=\"%d\"", config.CPUs))
	}
	if config.Memory != "" {
		additions = append(additions, fmt.Sprintf("ram=\"%s\"", config.Memory))
	}
	if config.DiskSize != "" {
		additions = append(additions, fmt.Sprintf("disk_size=\"%s\"", config.DiskSize))
	}
	if config.SSHPort > 0 {
		additions = append(additions, fmt.Sprintf("ssh_port=\"%d\"", config.SSHPort))
	}

	if len(additions) == 0 {
		return nil
	}

	f, err := os.OpenFile(confFile, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("cannot open conf file: %w", err)
	}
	defer f.Close()

	for _, line := range additions {
		if _, err := fmt.Fprintf(f, "\n%s", line); err != nil {
			return fmt.Errorf("cannot write to conf file: %w", err)
		}
	}

	return nil
}

// isRunning checks if a VM is running by looking for its PID file.
func (m *manager) isRunning(confFile string) bool {
	// quickemu stores PID in <vmdir>/<vmname>.pid
	// The vmdir is derived from the conf file name
	base := strings.TrimSuffix(filepath.Base(confFile), ".conf")
	pidFile := filepath.Join(m.workDir, base, base+".pid")

	data, err := os.ReadFile(pidFile)
	if err != nil {
		return false
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return false
	}

	// Check if process exists
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}

	// On Unix, FindProcess always succeeds. Send signal 0 to check.
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

// OutputJSON writes a value as indented JSON to stdout.
func OutputJSON(v any) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}
