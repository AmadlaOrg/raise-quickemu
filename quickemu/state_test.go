package quickemu

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func tempStatePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "state.json")
}

func TestStateManager_LoadAll_Empty(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	states, err := sm.LoadAll()
	require.NoError(t, err)
	assert.Empty(t, states)
}

func TestStateManager_AddAndGet(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))

	state := VMState{Name: "ubuntu-22.04", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", SSHPort: 22220, ConfFile: "/tmp/ubuntu-22.04.conf", CreatedAt: "2026-03-21T00:00:00Z"}
	require.NoError(t, sm.Add(state))

	got, err := sm.Get("ubuntu-22.04")
	require.NoError(t, err)
	assert.Equal(t, "ubuntu", got.OS)
	assert.Equal(t, "22.04", got.Release)
	assert.Equal(t, "running", got.State)
	assert.Equal(t, 22220, got.SSHPort)
}

func TestStateManager_Add_Duplicate(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))

	state := VMState{Name: "test", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}
	require.NoError(t, sm.Add(state))

	err := sm.Add(state)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestStateManager_Get_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	_, err := sm.Get("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_UpdateState(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))

	state := VMState{Name: "test", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}
	require.NoError(t, sm.Add(state))

	require.NoError(t, sm.UpdateState("test", "stopped"))

	got, err := sm.Get("test")
	require.NoError(t, err)
	assert.Equal(t, "stopped", got.State)
}

func TestStateManager_UpdateState_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	err := sm.UpdateState("nonexistent", "stopped")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_Remove(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))

	state := VMState{Name: "test", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/test.conf", CreatedAt: "2026-03-21T00:00:00Z"}
	require.NoError(t, sm.Add(state))

	require.NoError(t, sm.Remove("test"))

	_, err := sm.Get("test")
	assert.Error(t, err)
}

func TestStateManager_Remove_NotFound(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))
	err := sm.Remove("nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}

func TestStateManager_MultipleVMs(t *testing.T) {
	sm := NewStateManagerWithPath(tempStatePath(t))

	require.NoError(t, sm.Add(VMState{Name: "vm1", OS: "ubuntu", Release: "22.04", State: "running", Provider: "quickemu", ConfFile: "/tmp/vm1.conf", CreatedAt: "2026-03-21T00:00:00Z"}))
	require.NoError(t, sm.Add(VMState{Name: "vm2", OS: "debian", Release: "12", State: "stopped", Provider: "quickemu", ConfFile: "/tmp/vm2.conf", CreatedAt: "2026-03-21T01:00:00Z"}))

	states, err := sm.LoadAll()
	require.NoError(t, err)
	assert.Len(t, states, 2)
}

func TestStateManager_CorruptFile(t *testing.T) {
	path := tempStatePath(t)
	dir := filepath.Dir(path)
	require.NoError(t, os.MkdirAll(dir, 0o755))
	require.NoError(t, os.WriteFile(path, []byte("not json"), 0o644))

	sm := NewStateManagerWithPath(path)
	_, err := sm.LoadAll()
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to parse")
}
