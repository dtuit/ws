package command

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceUse_InitializesNamedWorkspaceState(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
workspaces:
  backend: repo-a
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	require.NoError(t, WorkspaceUse(m, wsHome, "backend", false))

	state, ok, err := loadWorkspaceContextState(wsHome, "backend")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "repo-a", state.Raw)
	assert.Equal(t, []string{"repo-a"}, state.Resolved)

	assertScopeEntriesForWorkspace(t, wsHome, "backend", "repo-a")
	assertWorkspaceFolders(t, workspaceFilePath(m, wsHome, "backend"), "~ workspace", "repo-a")
}

func TestSetContext_UsesActiveNamedWorkspace(t *testing.T) {
	t.Setenv("WS_WORKSPACE", "backend")

	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	require.NoError(t, SetContext(m, wsHome, "repo-b", false))

	state, ok, err := loadWorkspaceContextState(wsHome, "backend")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "repo-b", state.Raw)
	assert.Equal(t, []string{"repo-b"}, state.Resolved)

	_, ok, err = loadWorkspaceContextState(wsHome, "")
	require.NoError(t, err)
	assert.False(t, ok)

	assertScopeEntriesForWorkspace(t, wsHome, "backend", "repo-b")
	assertWorkspaceFolders(t, workspaceFilePath(m, wsHome, "backend"), "~ workspace", "repo-b")
}
