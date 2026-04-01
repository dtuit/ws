package command

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
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

func TestBuildWorkspaceJSON_IncludesGitWorktrees(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	wsHome := t.TempDir()
	repoDir := filepath.Join(wsHome, "repo")
	worktreeDir := filepath.Join(wsHome, "repo-feature")

	runGit(t, wsHome, "init", "repo")
	runGit(t, repoDir, "config", "user.name", "Test User")
	runGit(t, repoDir, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "README.md"), []byte("hello\n"), 0644))
	runGit(t, repoDir, "add", "README.md")
	runGit(t, repoDir, "commit", "-m", "init")
	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir)

	repos := []manifest.RepoInfo{
		{Name: "repo", Path: repoDir},
	}

	out, err := BuildWorkspaceJSON(repos, wsHome)
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))

	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)
	require.Len(t, folders, 3)

	first := folders[1].(map[string]interface{})
	assert.Equal(t, "repo", first["name"])
	assert.Equal(t, "repo", first["path"])

	second := folders[2].(map[string]interface{})
	assert.Equal(t, "repo [repo-feature]", second["name"])
	assert.Equal(t, "repo-feature", second["path"])
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

	filter, cmdArgs := ParseSuperArgs(m, []string{"ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
}

func TestParseSuperArgs_WithoutGroup(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs := ParseSuperArgs(m, []string{"git", "status"})
	assert.Equal(t, "", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
}

func TestParseSuperArgs_Empty(t *testing.T) {
	m, _ := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))

	filter, cmdArgs := ParseSuperArgs(m, nil)
	assert.Equal(t, "", filter)
	assert.Nil(t, cmdArgs)
}

func TestParseSuperArgs_AllFilter(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs := ParseSuperArgs(m, []string{"all", "git", "status"})
	assert.Equal(t, "all", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
}

func TestCompleteTopLevel(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`))
	require.NoError(t, err)

	result := Complete(m, []string{""}, 0)
	assert.Contains(t, result.Values, "ll")
	assert.Contains(t, result.Values, "backend")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "--workspace")
	assert.False(t, result.FallbackCommands)
}

func TestCompleteTopLevelFallsBackToCommands(t *testing.T) {
	result := Complete(nil, []string{"gi"}, 0)
	assert.Nil(t, result.Values)
	assert.True(t, result.FallbackCommands)
}

func TestCompleteCDRepos(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"cd", "repo"}, 1)
	assert.Equal(t, []string{"repo-a", "repo-b"}, result.Values)
}

func TestCompleteSetupIncludesFlagsAndFilters(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"setup", ""}, 1)
	assert.Contains(t, result.Values, "--install-shell")
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "all")
}

func TestCompleteCodeIncludesWorktreesFlagAndFilters(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"code", ""}, 1)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "ai")
}

func TestCompleteCodeAfterWorktreesFlagSuggestsFilters(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"code", "-t", ""}, 2)
	assert.Contains(t, result.Values, "ai")
	assert.NotContains(t, result.Values, "--worktrees")
}

func TestCompleteContextIncludesReset(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"context", ""}, 1)
	assert.Contains(t, result.Values, "add")
	assert.Contains(t, result.Values, "none")
	assert.Contains(t, result.Values, "reset")
}

func TestCompleteContextAddSuggestsFilters(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"context", "add", ""}, 2)
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
	assert.NotContains(t, result.Values, "reset")
}

func TestNormalizeContextFilter(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
  frontend: [repo-b]
repos:
  repo-a:
  repo-b:
  repo-c:
`))
	require.NoError(t, err)

	filter, err := normalizeContextFilter(m, "backend,repo-c,backend")
	require.NoError(t, err)
	assert.Equal(t, "backend,repo-c", filter)
}

func TestNormalizeContextFilter_AllWins(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, err := normalizeContextFilter(m, "backend,all")
	require.NoError(t, err)
	assert.Equal(t, "all", filter)
}

func TestNormalizeContextFilter_RejectsUnknown(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	_, err = normalizeContextFilter(m, "backend,nope")
	require.Error(t, err)
}

func TestAddContext_MergesWithExistingContext(t *testing.T) {
	wsHome := t.TempDir()
	m, err := manifest.Parse([]byte(`
root: repos
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
  frontend: [repo-b]
repos:
  repo-a:
  repo-b:
  repo-c:
`))
	require.NoError(t, err)

	require.NoError(t, AddContext(m, wsHome, "backend"))
	require.NoError(t, AddContext(m, wsHome, "repo-c,frontend"))

	data, err := os.ReadFile(filepath.Join(wsHome, contextFile))
	require.NoError(t, err)
	assert.Equal(t, "backend,repo-c,frontend\n", string(data))
}

func TestCompleteGroupCommandFallsBackToCommands(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	result := Complete(m, []string{"ai", ""}, 1)
	assert.True(t, result.FallbackCommands)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}
