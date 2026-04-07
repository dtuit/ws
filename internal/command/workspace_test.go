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

	out, err := BuildWorkspaceJSON(repos, "/workspace", false)
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))

	settings, ok := ws["settings"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, settings, "files.exclude")

	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)
	assert.Len(t, folders, 3)

	first := folders[0].(map[string]interface{})
	assert.Equal(t, "~ workspace", first["name"])
	assert.Equal(t, ".", first["path"])

	second := folders[1].(map[string]interface{})
	assert.Equal(t, "repo-a", second["name"])
	assert.Equal(t, "repos/repo-a", second["path"])
}

func TestBuildWorkspaceJSON_EmptyRepos(t *testing.T) {
	out, err := BuildWorkspaceJSON(nil, "/workspace", false)
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))
	folders := ws["folders"].([]interface{})
	assert.Len(t, folders, 1)
}

func TestBuildWorkspaceJSON_PerRepoRoots(t *testing.T) {
	repos := []manifest.RepoInfo{
		{Name: "default-repo", Path: "/workspace/../default-repo"},
		{Name: "vendor-repo", Path: "/workspace/vendor/vendor-repo"},
		{Name: "external-repo", Path: "/opt/external/external-repo"},
	}

	out, err := BuildWorkspaceJSON(repos, "/workspace", false)
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

	out, err := BuildWorkspaceJSON(repos, wsHome, true)
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

func TestWorkspaceSummary_WorktreesDisabled(t *testing.T) {
	assert.Equal(t,
		"Generated VS Code workspace ws.code-workspace (1 repo, worktrees disabled)",
		workspaceSummary("ws.code-workspace", 1, 3, false),
	)
}

func TestWorkspaceSummary_WorktreesEnabled(t *testing.T) {
	assert.Equal(t,
		"Generated VS Code workspace ws.code-workspace (2 repos, 3 worktrees)",
		workspaceSummary("ws.code-workspace", 2, 3, true),
	)
}

func TestCDPath_SelectsWorktreeByBasename(t *testing.T) {
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

	repo := manifest.RepoInfo{
		Name:   "repo",
		Path:   repoDir,
		Branch: "master",
	}

	path, err := CDPath(repo, "repo-feature")
	require.NoError(t, err)
	assert.Equal(t, worktreeDir, path)
}
