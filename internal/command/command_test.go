package command

import (
	"encoding/json"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildWorkspaceJSON(t *testing.T) {
	repos := []manifest.RepoInfo{
		{Name: "repo-a", Path: "/workspace/repos/repo-a"},
		{Name: "repo-b", Path: "/workspace/repos/repo-b"},
	}

	out, err := BuildWorkspaceJSON(repos, "/workspace")
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))

	// Has default settings
	settings, ok := ws["settings"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, settings, "files.exclude")

	// Folders: workspace + 2 repos
	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)
	assert.Len(t, folders, 3)

	// First folder is workspace root
	first := folders[0].(map[string]interface{})
	assert.Equal(t, "~ workspace", first["name"])
	assert.Equal(t, ".", first["path"])

	// Repo folders have correct paths
	second := folders[1].(map[string]interface{})
	assert.Equal(t, "repo-a", second["name"])
	assert.Equal(t, "repos/repo-a", second["path"])
}

func TestBuildWorkspaceJSON_EmptyRepos(t *testing.T) {
	out, err := BuildWorkspaceJSON(nil, "/workspace")
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))
	folders := ws["folders"].([]interface{})
	assert.Len(t, folders, 1) // just workspace root
}

func TestBuildWorkspaceJSON_PerRepoRoots(t *testing.T) {
	repos := []manifest.RepoInfo{
		{Name: "default-repo", Path: "/workspace/../default-repo"},
		{Name: "vendor-repo", Path: "/workspace/vendor/vendor-repo"},
		{Name: "external-repo", Path: "/opt/external/external-repo"},
	}

	out, err := BuildWorkspaceJSON(repos, "/workspace")
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))
	folders := ws["folders"].([]interface{})

	assert.Equal(t, "../default-repo", folders[1].(map[string]interface{})["path"])
	assert.Equal(t, "vendor/vendor-repo", folders[2].(map[string]interface{})["path"])
	assert.Equal(t, "../opt/external/external-repo", folders[3].(map[string]interface{})["path"])
}

func TestParseSuperArgs_WithGroup(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs, includeWorktrees := ParseSuperArgs(m, []string{"ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.False(t, includeWorktrees)
}

func TestParseSuperArgs_WithoutGroup(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs, includeWorktrees := ParseSuperArgs(m, []string{"git", "status"})
	assert.Equal(t, "", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.False(t, includeWorktrees)
}

func TestParseSuperArgs_WorktreesBeforeFilter(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs, includeWorktrees := ParseSuperArgs(m, []string{"--worktrees", "ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, includeWorktrees)
}

func TestParseSuperArgs_WorktreesAfterFilter(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs, includeWorktrees := ParseSuperArgs(m, []string{"ai", "--worktrees", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, includeWorktrees)
}

func TestParseSuperArgs_Empty(t *testing.T) {
	m, _ := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))

	filter, cmdArgs, includeWorktrees := ParseSuperArgs(m, nil)
	assert.Equal(t, "", filter)
	assert.Nil(t, cmdArgs)
	assert.False(t, includeWorktrees)
}
