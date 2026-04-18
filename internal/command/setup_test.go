package command

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetupGuide_ListsGroupsWithCounts(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  backend: [api-server, auth-service, worker]
  frontend: [web-app, admin-ui]
repos:
  api-server:
  auth-service:
  worker:
  web-app:
  admin-ui:
  standalone:
`)
	require.NoError(t, err)

	out := captureCommandStdout(t, func() {
		require.NoError(t, SetupGuide(m, wsHome))
	})

	// Active repo count — standalone is in manifest but no group
	assert.Contains(t, out, "6 active repos")
	assert.Contains(t, out, "2 groups")

	// Both groups appear with their counts
	assert.Contains(t, out, "backend")
	assert.Contains(t, out, "(3)")
	assert.Contains(t, out, "frontend")
	assert.Contains(t, out, "(2)")

	// Sample members rendered
	assert.Contains(t, out, "api-server")
	assert.Contains(t, out, "web-app")

	// Guides toward the context flow, not a specific group
	assert.Contains(t, out, "ws context <group>")
	assert.Contains(t, out, "ws setup all")
}

func TestSetupGuide_EmptyGroupsOmitted(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  empty: []
  backend: [api-server]
repos:
  api-server:
`)
	require.NoError(t, err)

	out := captureCommandStdout(t, func() {
		require.NoError(t, SetupGuide(m, wsHome))
	})

	assert.Contains(t, out, "backend")
	assert.NotContains(t, out, "empty")
	// "1 groups" because the empty one was filtered
	assert.Contains(t, out, "1 groups")
}

func TestSetupGuide_NoGroups(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  solo:
`)
	require.NoError(t, err)

	out := captureCommandStdout(t, func() {
		require.NoError(t, SetupGuide(m, wsHome))
	})

	assert.Contains(t, out, "1 active repo")
	assert.Contains(t, out, "No groups defined")
	assert.Contains(t, out, "ws setup all")
}

func TestSetupGuide_NoActiveRepos(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
`)
	require.NoError(t, err)

	out := captureCommandStdout(t, func() {
		require.NoError(t, SetupGuide(m, wsHome))
	})

	assert.Contains(t, strings.ToLower(out), "no active repos")
}

func TestSetupGuide_SampleTruncation(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  big: [a, b, c, d, e, f]
repos:
  a: {}
  b: {}
  c: {}
  d: {}
  e: {}
  f: {}
`)
	require.NoError(t, err)

	out := captureCommandStdout(t, func() {
		require.NoError(t, SetupGuide(m, wsHome))
	})

	// First 3 members shown, then +N overflow
	assert.Contains(t, out, "a, b, c, +3")
	assert.NotContains(t, out, "d, e, f")
}

func TestSyncScopeDir_SkipsUnclonedRepos(t *testing.T) {
	wsHome := t.TempDir()

	clonedPath := filepath.Join(wsHome, "repos", "cloned")
	initCheckout(t, clonedPath)
	unclonedPath := filepath.Join(wsHome, "repos", "uncloned")

	repos := []manifest.RepoInfo{
		{Name: "cloned", Path: clonedPath},
		{Name: "uncloned", Path: unclonedPath},
	}

	require.NoError(t, syncScopeDir(wsHome, ".scope", repos))

	entries, err := os.ReadDir(filepath.Join(wsHome, ".scope"))
	require.NoError(t, err)

	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	assert.Equal(t, []string{"cloned"}, names)
}

func TestWriteWorkspace_SkipsUnclonedRepos(t *testing.T) {
	wsHome := t.TempDir()

	clonedPath := filepath.Join(wsHome, "repos", "cloned")
	initCheckout(t, clonedPath)
	unclonedPath := filepath.Join(wsHome, "repos", "uncloned")

	m, err := parseManifestYAML(`
workspace: ws.code-workspace
root: repos
remotes:
  default: git@example.com
repos:
  cloned:
  uncloned:
`)
	require.NoError(t, err)

	repos := []manifest.RepoInfo{
		{Name: "cloned", Path: clonedPath},
		{Name: "uncloned", Path: unclonedPath},
	}

	require.NoError(t, writeWorkspace(m, wsHome, repos, false))

	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "cloned")
}
