package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestRemoveContext_SubtractsFromCurrentScope(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
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
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-c"))

	require.NoError(t, SetContext(m, wsHome, "backend,repo-c,frontend", false))
	require.NoError(t, RemoveContext(m, wsHome, "frontend,repo-c", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "repo-a", state.Raw)
	assert.Equal(t, []string{"repo-a"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo-a")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "repo-a")
}

func TestRemoveContext_FromResolvedAllScope(t *testing.T) {
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
	require.NoError(t, RemoveContext(m, wsHome, "local-repo", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "repo-a", state.Raw)
	assert.Equal(t, []string{"repo-a"}, state.Resolved)
}

func TestRemoveContext_WorktreeTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo:
`)
	require.NoError(t, err)

	repoDir := filepath.Join(wsHome, "repos", "repo")
	worktreeDir := filepath.Join(wsHome, "repo-feature")
	initCheckout(t, repoDir)
	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir)

	require.NoError(t, SetContext(m, wsHome, "repo", true))
	require.NoError(t, RemoveContext(m, wsHome, "repo@repo-feature", true))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "repo", state.Raw)
	assert.Equal(t, []string{"repo"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "repo")
}

func TestRemoveContext_RequiresExistingContext(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	err = RemoveContext(m, wsHome, "repo-a", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no context set")
}

func TestRemoveContext_RejectsEmptyResult(t *testing.T) {
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
	require.NoError(t, SetContext(m, wsHome, "repo-a", false))

	err = RemoveContext(m, wsHome, "repo-a", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "leave the context empty")
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

	repos, err := resolveContextRepos(m, wsHome, "local-repo", false)
	require.NoError(t, err)
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

	repos, err := resolveContextRepos(m, wsHome, "all", false)
	require.NoError(t, err)
	assert.Equal(t, []string{"local-repo", "repo-a"}, repoNames(repos))
}

func TestResolveCommandRepos_ExplicitWorktreeTarget(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}

	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
remotes:
  default: git@example.com
repos:
  repo:
`)
	require.NoError(t, err)

	repoDir := filepath.Join(wsHome, "repos", "repo")
	worktreeDir := filepath.Join(wsHome, "repo-feature")
	initCheckout(t, repoDir)
	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir)

	repos, err := resolveCommandRepos(m, wsHome, "repo@repo-feature", false)
	require.NoError(t, err)
	require.Len(t, repos, 1)
	assert.Equal(t, "repo@repo-feature", repos[0].Name)
	assert.Equal(t, "repo-feature", repos[0].Worktree)
	assert.Equal(t, worktreeDir, repos[0].Path)
}
