package command

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
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
	out, err := BuildWorkspaceJSON(nil, "/workspace", false)
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

func TestParseSuperArgs_WithGroup(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.False(t, worktrees.Set)
}

func TestParseSuperArgs_WithoutGroup(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"git", "status"})
	assert.Equal(t, "", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.False(t, worktrees.Set)
}

func TestParseSuperArgs_WorktreesBeforeFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"--worktrees", "ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, worktrees.Set)
	assert.True(t, worktrees.Value)
}

func TestParseSuperArgs_ShorthandWorktreesBeforeFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"-t", "ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, worktrees.Set)
	assert.True(t, worktrees.Value)
}

func TestParseSuperArgs_WorktreesAfterFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"ai", "--worktrees", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, worktrees.Set)
	assert.True(t, worktrees.Value)
}

func TestParseSuperArgs_NoWorktreesAfterFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"ai", "--no-worktrees", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.True(t, worktrees.Set)
	assert.False(t, worktrees.Value)
}

func TestParseSuperArgs_Empty(t *testing.T) {
	m, _ := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, nil)
	assert.Equal(t, "", filter)
	assert.Nil(t, cmdArgs)
	assert.False(t, worktrees.Set)
}

func TestParseSuperArgs_AllFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	filter, cmdArgs, worktrees := ParseSuperArgs(m, []string{"all", "git", "status"})
	assert.Equal(t, "all", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
	assert.False(t, worktrees.Set)
}

func TestCompleteTopLevel(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	result := Complete(m, []string{""}, 0)
	assert.Contains(t, result.Values, "ll")
	assert.Contains(t, result.Values, "open")
	assert.Contains(t, result.Values, "backend")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "--workspace")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
	assert.False(t, result.FallbackCommands)
}

func TestCompleteTopLevelFallsBackToCommands(t *testing.T) {
	result := Complete(nil, []string{"gi"}, 0)
	assert.Nil(t, result.Values)
	assert.True(t, result.FallbackCommands)
}

func TestCompleteCDRepos(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"cd", "repo"}, 1)
	assert.Equal(t, []string{"repo-a", "repo-b"}, result.Values)
}

func TestCompleteSetupIncludesFlagsAndFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"setup", ""}, 1)
	assert.Contains(t, result.Values, "--install-shell")
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "all")
}

func TestCompleteLLIncludesWorktreesFlagAndFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"ll", ""}, 1)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "-W")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "ai")
}

func TestCompleteListIncludesWorktreesFlagAndShowAll(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"list", ""}, 1)
	assert.Contains(t, result.Values, "--all")
	assert.Contains(t, result.Values, "-a")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
}

func TestCompletePassthroughAfterWorktreesFallsBackToCommands(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"--", "--worktrees", "gi"}, 2)
	assert.True(t, result.FallbackCommands)
}

func TestCompletePassthroughAfterNoWorktreesFallsBackToCommands(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"--", "--no-worktrees", "gi"}, 2)
	assert.True(t, result.FallbackCommands)
}

func TestCompletePassthroughAfterShorthandWorktreesFallsBackToCommands(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"--", "-t", "gi"}, 2)
	assert.True(t, result.FallbackCommands)
}

func TestCompleteContextIncludesReset(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", ""}, 1)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "add")
	assert.Contains(t, result.Values, "none")
	assert.Contains(t, result.Values, "reset")
}

func TestCompleteContextAfterWorktreesFlagIncludesFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", "-t", ""}, 2)
	assert.Contains(t, result.Values, "add")
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
}

func TestCompleteContextAddSuggestsFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", "add", ""}, 2)
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.NotContains(t, result.Values, "reset")
}

func TestNormalizeContextFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
  frontend: [repo-b]
repos:
  repo-a:
  repo-b:
  repo-c:
`)
	require.NoError(t, err)

	filter, err := normalizeContextFilter(m, "backend,repo-c,backend")
	require.NoError(t, err)
	assert.Equal(t, "backend,repo-c", filter)
}

func TestNormalizeContextFilter_AllWins(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	filter, err := normalizeContextFilter(m, "repo-b,all")
	require.NoError(t, err)
	assert.Equal(t, "all", filter)
}

func TestNormalizeContextFilter_RejectsUnknown(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	_, err = normalizeContextFilter(m, "backend,nope")
	require.Error(t, err)
}

func TestAddContext_MergesWithExistingContext(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
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
`)
	require.NoError(t, err)

	require.NoError(t, SetContext(m, wsHome, "backend", false))
	require.NoError(t, AddContext(m, wsHome, "repo-c,frontend", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "backend,repo-c,frontend", state.Raw)
	assert.Equal(t, []string{"repo-a", "repo-c", "repo-b"}, state.Resolved)
}

func TestAddContext_NoExistingContextBehavesLikeSetContext(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  backend: [repo-a, repo-b]
repos:
  repo-a:
  repo-b:
`, `
repos:
  local-repo:
`)

	require.NoError(t, AddContext(m, wsHome, "local-repo", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "local-repo", state.Raw)
	assert.Equal(t, []string{"local-repo"}, state.Resolved)
	assertScopeEntries(t, wsHome, "local-repo")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "local-repo")
}

func TestSetContextAll_UsesOnlyClonedReposInResolvedScope(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`, `
repos:
  local-repo:
`)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "local-repo"))

	require.NoError(t, SetContext(m, wsHome, "all", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "all", state.Raw)
	assert.Equal(t, []string{"local-repo", "repo-a"}, state.Resolved)

	filter, ok := GetDefaultContext(wsHome)
	require.True(t, ok)
	assert.Equal(t, "local-repo,repo-a", filter)
	assertScopeEntries(t, wsHome, "local-repo", "repo-a")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "local-repo", "repo-a")
}

func TestSetContextNone_UsesOnlyClonedReposInResolvedScope(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`, `
repos:
  local-repo:
`)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "local-repo"))

	require.NoError(t, SetContext(m, wsHome, "none", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "", state.Raw)
	assert.Equal(t, []string{"local-repo", "repo-a"}, state.Resolved)

	filter, ok := GetDefaultContext(wsHome)
	require.True(t, ok)
	assert.Equal(t, "local-repo,repo-a", filter)
	assertScopeEntries(t, wsHome, "local-repo", "repo-a")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "local-repo", "repo-a")
}

func TestSetContextFailureDoesNotOverwriteResolvedScope(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  empty: []
repos:
  repo-a:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))

	require.NoError(t, SetContext(m, wsHome, "all", false))

	err = SetContext(m, wsHome, "empty", false)
	require.Error(t, err)

	filter, ok := GetDefaultContext(wsHome)
	require.True(t, ok)
	assert.Equal(t, "repo-a", filter)
	assertScopeEntries(t, wsHome, "repo-a")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "repo-a")
}

func TestSetContext_RemovesLegacyResolvedContextFile(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, legacyResolvedContextFile), []byte("repo-a\n"), 0644))

	require.NoError(t, SetContext(m, wsHome, "all", false))

	_, err = os.Stat(filepath.Join(wsHome, legacyResolvedContextFile))
	assert.True(t, os.IsNotExist(err))
}

func TestResolveContextRepos_ExplicitLocalRepoStillIncluded(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
`, `
repos:
  local-repo:
groups:
  local: [local-repo]
`)

	repos := resolveContextRepos(m, wsHome, "local-repo")
	require.Len(t, repos, 1)
	assert.Equal(t, "local-repo", repos[0].Name)
}

func TestResolveContextRepos_AllIncludesOnlyClonedRepos(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`, `
repos:
  local-repo:
`)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "local-repo"))

	repos := resolveContextRepos(m, wsHome, "all")
	assert.Equal(t, []string{"local-repo", "repo-a"}, repoNames(repos))
}

func TestCompleteGroupCommandFallsBackToCommands(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"ai", ""}, 1)
	assert.True(t, result.FallbackCommands)
}

func TestCompleteGroupCommandDelegatesAfterCommandWord(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"ai", "git", ""}, 2)
	assert.True(t, result.DelegateCommands)
	assert.Equal(t, 1, result.DelegateStart)
}

func TestCompletePassthroughDelegatesAfterCommandWord(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"git", "branch", ""}, 2)
	assert.True(t, result.DelegateCommands)
	assert.Equal(t, 0, result.DelegateStart)
}

func TestCompleteEscapedPassthroughDelegatesAfterCommandWord(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"--", "ai", "git", "branch", ""}, 4)
	assert.True(t, result.DelegateCommands)
	assert.Equal(t, 2, result.DelegateStart)
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func parseManifestYAML(yaml string) (*manifest.Manifest, error) {
	if !strings.Contains(yaml, "\nroot:") && !strings.HasPrefix(strings.TrimSpace(yaml), "root:") {
		yaml = "root: repos\n" + yaml
	}
	return manifest.Parse([]byte(yaml))
}

func readStoredContext(t *testing.T, wsHome string) contextState {
	t.Helper()

	data, err := os.ReadFile(filepath.Join(wsHome, contextFile))
	require.NoError(t, err)

	var state contextState
	require.NoError(t, yaml.Unmarshal(data, &state))
	return state
}

func loadManifestWithLocal(t *testing.T, wsHome, manifestYAML, localYAML string) *manifest.Manifest {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(manifestYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.local.yml"), []byte(localYAML), 0644))

	m, err := manifest.LoadWithLocal(wsHome)
	require.NoError(t, err)
	return m
}

func initCheckout(t *testing.T, repoPath string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(repoPath), 0755))
	runGit(t, filepath.Dir(repoPath), "init", filepath.Base(repoPath))
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0644))
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "init")
}

func assertScopeEntries(t *testing.T, wsHome string, want ...string) {
	t.Helper()

	entries, err := os.ReadDir(filepath.Join(wsHome, scopeDir))
	require.NoError(t, err)

	var got []string
	for _, entry := range entries {
		got = append(got, entry.Name())
	}
	assert.Equal(t, want, got)
}

func assertWorkspaceFolders(t *testing.T, workspacePath string, want ...string) {
	t.Helper()

	data, err := os.ReadFile(workspacePath)
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &ws))

	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)

	var got []string
	for _, raw := range folders {
		folder, ok := raw.(map[string]interface{})
		require.True(t, ok)
		name, ok := folder["name"].(string)
		require.True(t, ok)
		got = append(got, name)
	}

	assert.Equal(t, want, got)
}

func repoNames(repos []manifest.RepoInfo) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}
