package quickemu

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeExecCommand(exitCode int, stdout string) func(string, ...string) *exec.Cmd {
	return func(command string, args ...string) *exec.Cmd {
		cs := []string{"-test.run=TestHelperProcess", "--", command}
		cs = append(cs, args...)
		cmd := exec.Command(os.Args[0], cs...)
		cmd.Env = append(os.Environ(),
			"GO_WANT_HELPER_PROCESS=1",
			fmt.Sprintf("GO_HELPER_EXIT_CODE=%d", exitCode),
			fmt.Sprintf("GO_HELPER_STDOUT=%s", stdout),
		)
		return cmd
	}
}

func TestHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_PROCESS") != "1" {
		return
	}

	exitCode := 0
	if code := os.Getenv("GO_HELPER_EXIT_CODE"); code != "" {
		fmt.Sscanf(code, "%d", &exitCode)
	}

	stdout := os.Getenv("GO_HELPER_STDOUT")
	if stdout != "" {
		fmt.Fprint(os.Stdout, stdout)
	}

	os.Exit(exitCode)
}

func TestUp_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()

	// quickget and quickemu both succeed
	ExecCommand = fakeExecCommand(0, "")

	workDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, workDir)

	// Create a fake .conf file that quickget would have created
	confFile := filepath.Join(workDir, "ubuntu-22.04.conf")
	require.NoError(t, os.WriteFile(confFile, []byte("guest_os=\"linux\"\n"), 0o644))

	config := &VMConfig{
		OS:      "ubuntu",
		Release: "22.04",
		CPUs:    2,
		Memory:  "4G",
	}

	state, err := mgr.Up(config)
	require.NoError(t, err)
	assert.Equal(t, "ubuntu-22.04", state.Name)
	assert.Equal(t, "ubuntu", state.OS)
	assert.Equal(t, "22.04", state.Release)
	assert.Equal(t, "running", state.State)
	assert.Equal(t, "quickemu", state.Provider)
	assert.Equal(t, 22220, state.SSHPort)
	assert.NotEmpty(t, state.CreatedAt)
}

func TestUp_WithEdition(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()

	ExecCommand = fakeExecCommand(0, "")

	workDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, workDir)

	confFile := filepath.Join(workDir, "debian-12-kde.conf")
	require.NoError(t, os.WriteFile(confFile, []byte("guest_os=\"linux\"\n"), 0o644))

	config := &VMConfig{
		OS:      "debian",
		Release: "12",
		Edition: "kde",
	}

	state, err := mgr.Up(config)
	require.NoError(t, err)
	assert.Equal(t, "debian-12-kde", state.Name)
}

func TestUp_MissingOS(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, t.TempDir())

	config := &VMConfig{Release: "22.04"}

	_, err := mgr.Up(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "OS is required")
}

func TestUp_MissingRelease(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, t.TempDir())

	config := &VMConfig{OS: "ubuntu"}

	_, err := mgr.Up(config)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "release is required")
}

func TestUp_Defaults(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, "")

	workDir := t.TempDir()
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, workDir)

	confFile := filepath.Join(workDir, "ubuntu-22.04.conf")
	require.NoError(t, os.WriteFile(confFile, []byte("guest_os=\"linux\"\n"), 0o644))

	config := &VMConfig{OS: "ubuntu", Release: "22.04"}
	state, err := mgr.Up(config)
	require.NoError(t, err)
	assert.Equal(t, 22220, state.SSHPort)
}

func TestHalt_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, "")

	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	require.NoError(t, sm.Add(VMState{Name: "test-vm", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}))

	mgr := NewWithState(sm, t.TempDir())
	err := mgr.Halt("test-vm")
	require.NoError(t, err)

	got, err := sm.Get("test-vm")
	require.NoError(t, err)
	assert.Equal(t, "stopped", got.State)
}

func TestHalt_NotFound(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, t.TempDir())

	err := mgr.Halt("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestDestroy_Success(t *testing.T) {
	origExecCommand := ExecCommand
	defer func() { ExecCommand = origExecCommand }()
	ExecCommand = fakeExecCommand(0, "")

	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	require.NoError(t, sm.Add(VMState{Name: "test-vm", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}))

	mgr := NewWithState(sm, t.TempDir())
	err := mgr.Destroy("test-vm")
	require.NoError(t, err)

	_, err = sm.Get("test-vm")
	assert.Error(t, err)
}

func TestStatus_Stopped(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	require.NoError(t, sm.Add(VMState{Name: "test-vm", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", SSHPort: 22220, ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}))

	mgr := NewWithState(sm, t.TempDir())
	state, err := mgr.Status("test-vm")
	require.NoError(t, err)
	assert.Equal(t, "test-vm", state.Name)
	assert.Equal(t, "stopped", state.State) // No PID file = stopped
	assert.Equal(t, 22220, state.SSHPort)
}

func TestStatusAll_Empty(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	mgr := NewWithState(sm, t.TempDir())

	states, err := mgr.StatusAll()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestStatusAll_Multiple(t *testing.T) {
	statePath := filepath.Join(t.TempDir(), "state.json")
	sm := NewStateManagerWithPath(statePath)
	require.NoError(t, sm.Add(VMState{Name: "vm1", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/vm1.conf", CreatedAt: "2026-03-21T00:00:00Z"}))
	require.NoError(t, sm.Add(VMState{Name: "vm2", OS: "debian", Release: "12", State: "running", Provider: "quickemu", ConfFile: "/tmp/vm2.conf", CreatedAt: "2026-03-21T01:00:00Z"}))

	mgr := NewWithState(sm, t.TempDir())
	states, err := mgr.StatusAll()
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestConfFilePath(t *testing.T) {
	mgr := &manager{workDir: "/tmp/vms"}

	config := &VMConfig{OS: "ubuntu", Release: "22.04"}
	assert.Equal(t, "/tmp/vms/ubuntu-22.04.conf", mgr.confFilePath(config))

	config = &VMConfig{OS: "debian", Release: "12", Edition: "kde"}
	assert.Equal(t, "/tmp/vms/debian-12-kde.conf", mgr.confFilePath(config))
}

func TestCustomizeConf(t *testing.T) {
	mgr := &manager{workDir: t.TempDir()}

	confFile := filepath.Join(mgr.workDir, "test.conf")
	require.NoError(t, os.WriteFile(confFile, []byte("guest_os=\"linux\"\n"), 0o644))

	config := &VMConfig{
		CPUs:     4,
		Memory:   "8G",
		DiskSize: "64G",
		SSHPort:  22222,
	}

	err := mgr.customizeConf(confFile, config)
	require.NoError(t, err)

	data, err := os.ReadFile(confFile)
	require.NoError(t, err)
	content := string(data)

	assert.Contains(t, content, `cpu_cores="4"`)
	assert.Contains(t, content, `ram="8G"`)
	assert.Contains(t, content, `disk_size="64G"`)
	assert.Contains(t, content, `ssh_port="22222"`)
}

func TestIsRunning_NoPidFile(t *testing.T) {
	mgr := &manager{workDir: t.TempDir()}
	assert.False(t, mgr.isRunning("/tmp/nonexistent.conf"))
}

func TestIsRunning_InvalidPid(t *testing.T) {
	workDir := t.TempDir()
	vmDir := filepath.Join(workDir, "test")
	require.NoError(t, os.MkdirAll(vmDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "test.pid"), []byte("notanumber"), 0o644))

	mgr := &manager{workDir: workDir}
	assert.False(t, mgr.isRunning(filepath.Join(workDir, "test.conf")))
}

func TestIsRunning_DeadProcess(t *testing.T) {
	workDir := t.TempDir()
	vmDir := filepath.Join(workDir, "test")
	require.NoError(t, os.MkdirAll(vmDir, 0o755))
	// Use PID 99999999 which almost certainly doesn't exist
	require.NoError(t, os.WriteFile(filepath.Join(vmDir, "test.pid"), []byte("99999999"), 0o644))

	mgr := &manager{workDir: workDir}
	assert.False(t, mgr.isRunning(filepath.Join(workDir, "test.conf")))
}
