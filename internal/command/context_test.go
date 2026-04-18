package command

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/dtuit/ws/internal/manifest"
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

func TestNormalizeContextFilter_RejectsInvalidActivityFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	for _, filter := range []string{"mine", "dirty:1d", "active:0", "mine:soon"} {
		_, err = normalizeContextFilter(m, filter)
		require.Error(t, err)
	}
}

func TestNormalizeContextFilter_AllowsActivityFilters(t *testing.T) {
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

	filter, err := normalizeContextFilter(m, "backend,active,dirty,mine:1d,active:1d,active")
	require.NoError(t, err)
	assert.Equal(t, "backend,active,dirty,mine:1d,active:1d", filter)
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

func TestSetContext_PrintsResolvedRepos(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	output := captureCommandStdout(t, func() {
		require.NoError(t, SetContext(m, wsHome, "backend,repo-b", false))
	})

	assert.Contains(t, output, `Context set to "backend,repo-b" (2 repos)`)
	assert.Contains(t, output, "Resolved: repo-a, repo-b")
}

func TestSetContext_UsesConfiguredScopeDirs(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
scopes:
  - dir: .scope
    source: context
  - dir: .all-repos
    source: all
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
  repo-c:
`)
	require.NoError(t, err)

	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	require.NoError(t, SetContext(m, wsHome, "repo-a", false))

	assertScopeEntriesInDir(t, wsHome, ".scope", "repo-a")
	assertScopeEntriesInDir(t, wsHome, ".all-repos", "repo-a", "repo-b")
}

func TestSetContext_CanDisableScopeDirs(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
scopes: []
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))

	require.NoError(t, SetContext(m, wsHome, "repo-a", false))

	assertNoScopeDir(t, wsHome, manifest.DefaultScopeDir)
}

func TestSetContext_CustomScopesClearLegacyDefaultScope(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
scopes:
  - dir: .scoped
    source: context
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	require.NoError(t, os.MkdirAll(filepath.Join(wsHome, manifest.DefaultScopeDir), 0755))
	require.NoError(t, os.Symlink("../repos/repo-a", filepath.Join(wsHome, manifest.DefaultScopeDir, "repo-a")))

	require.NoError(t, SetContext(m, wsHome, "repo-a", false))

	assertScopeEntriesInDir(t, wsHome, ".scoped", "repo-a")
	assertScopeEntriesInDir(t, wsHome, manifest.DefaultScopeDir)
}

func TestShowContext_PrintsResolvedRepos(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "repo-a,repo-b", []manifest.RepoInfo{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}, nil))

	output := captureCommandStdout(t, func() {
		ShowContext(m, wsHome)
	})

	assert.Contains(t, output, "Context: repo-a,repo-b (2 repos)")
	assert.Contains(t, output, "Resolved: repo-a, repo-b")
}

func TestRefreshContext_RequiresExistingContext(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	err = RefreshContext(m, wsHome, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no context set")
}

func TestRefreshContext_ReresolvesDynamicFilter(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	repoA := filepath.Join(wsHome, "repos", "repo-a")
	repoB := filepath.Join(wsHome, "repos", "repo-b")
	initCheckout(t, repoA)
	initCheckout(t, repoB)

	runGit(t, repoA, "config", "user.name", "Other User")
	runGit(t, repoA, "config", "user.email", "other@example.com")
	runGit(t, repoB, "config", "user.name", "Other User")
	runGit(t, repoB, "config", "user.email", "other@example.com")

	require.NoError(t, os.WriteFile(filepath.Join(repoA, "dirty.txt"), []byte("dirty\n"), 0644))
	require.NoError(t, SetContext(m, wsHome, dirtyFilterToken, false))

	require.NoError(t, os.Remove(filepath.Join(repoA, "dirty.txt")))
	require.NoError(t, os.WriteFile(filepath.Join(repoB, "dirty.txt"), []byte("dirty\n"), 0644))

	output := captureCommandStdout(t, func() {
		require.NoError(t, RefreshContext(m, wsHome, false))
	})

	state := readStoredContext(t, wsHome)
	assert.Equal(t, dirtyFilterToken, state.Raw)
	assert.Equal(t, []string{"repo-b"}, state.Resolved)
	assert.Contains(t, output, `Context refreshed from "dirty" (1 repos)`)
	assert.Contains(t, output, "Resolved: repo-b")
}

func TestRefreshContext_ReresolvesWorktrees(t *testing.T) {
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

	require.NoError(t, SetContext(m, wsHome, "repo", true))

	runGit(t, repoDir, "worktree", "add", "-b", "feature", worktreeDir)

	output := captureCommandStdout(t, func() {
		require.NoError(t, RefreshContext(m, wsHome, true))
	})

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "repo", state.Raw)
	assert.Equal(t, []string{"repo", "repo@repo-feature"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo", "repo@repo-feature")
	assert.Contains(t, output, `Context refreshed from "repo" (2 repos)`)
	assert.Contains(t, output, "Resolved: repo, repo@repo-feature")
}

func TestSetContext_ActiveIncludesDirtyAndRecentRepos(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
  repo-c:
`)
	require.NoError(t, err)

	repoA := filepath.Join(wsHome, "repos", "repo-a")
	repoB := filepath.Join(wsHome, "repos", "repo-b")
	repoC := filepath.Join(wsHome, "repos", "repo-c")
	initCheckout(t, repoA)
	initCheckout(t, repoB)
	initCheckout(t, repoC)

	runGit(t, repoA, "config", "user.name", "Other User")
	runGit(t, repoA, "config", "user.email", "other@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "dirty.txt"), []byte("dirty\n"), 0644))

	runGit(t, repoB, "config", "user.name", "Local User")
	runGit(t, repoB, "config", "user.email", "local@example.com")
	commitEmptyAt(t, repoB, "recent work", "Local User", "local@example.com", time.Now().Add(-48*time.Hour))

	runGit(t, repoC, "config", "user.name", "Other User")
	runGit(t, repoC, "config", "user.email", "other@example.com")

	require.NoError(t, SetContext(m, wsHome, activeFilterToken, false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, activeFilterToken, state.Raw)
	assert.Equal(t, []string{"repo-a", "repo-b"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo-a", "repo-b")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "repo-a", "repo-b")
}

func TestSetContext_ActiveDurationRestrictsRecentMatches(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
  repo-c:
`)
	require.NoError(t, err)

	repoA := filepath.Join(wsHome, "repos", "repo-a")
	repoB := filepath.Join(wsHome, "repos", "repo-b")
	repoC := filepath.Join(wsHome, "repos", "repo-c")
	initCheckout(t, repoA)
	initCheckout(t, repoB)
	initCheckout(t, repoC)

	runGit(t, repoA, "config", "user.name", "Other User")
	runGit(t, repoA, "config", "user.email", "other@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "dirty.txt"), []byte("dirty\n"), 0644))

	runGit(t, repoB, "config", "user.name", "Local User")
	runGit(t, repoB, "config", "user.email", "local@example.com")
	commitEmptyAt(t, repoB, "recent work", "Local User", "local@example.com", time.Now().Add(-48*time.Hour))

	runGit(t, repoC, "config", "user.name", "Other User")
	runGit(t, repoC, "config", "user.email", "other@example.com")

	require.NoError(t, SetContext(m, wsHome, "active:1d", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "active:1d", state.Raw)
	assert.Equal(t, []string{"repo-a"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo-a")
}

func TestSetContext_MineDurationIncludesOnlyRecentLocalCommits(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	repoA := filepath.Join(wsHome, "repos", "repo-a")
	repoB := filepath.Join(wsHome, "repos", "repo-b")
	initCheckout(t, repoA)
	initCheckout(t, repoB)

	runGit(t, repoA, "config", "user.name", "Local User")
	runGit(t, repoA, "config", "user.email", "local@example.com")
	commitEmptyAt(t, repoA, "recent work", "Local User", "local@example.com", time.Now().Add(-12*time.Hour))
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "dirty.txt"), []byte("dirty\n"), 0644))

	runGit(t, repoB, "config", "user.name", "Other User")
	runGit(t, repoB, "config", "user.email", "other@example.com")

	require.NoError(t, SetContext(m, wsHome, "mine:1d", false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "mine:1d", state.Raw)
	assert.Equal(t, []string{"repo-a"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo-a")
}

func TestAddContext_ActiveCombinesWithExistingContext(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
  repo-c:
`)
	require.NoError(t, err)

	repoA := filepath.Join(wsHome, "repos", "repo-a")
	repoB := filepath.Join(wsHome, "repos", "repo-b")
	repoC := filepath.Join(wsHome, "repos", "repo-c")
	initCheckout(t, repoA)
	initCheckout(t, repoB)
	initCheckout(t, repoC)

	runGit(t, repoA, "config", "user.name", "Other User")
	runGit(t, repoA, "config", "user.email", "other@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoA, "dirty.txt"), []byte("dirty\n"), 0644))

	runGit(t, repoB, "config", "user.name", "Local User")
	runGit(t, repoB, "config", "user.email", "local@example.com")
	commitEmptyAt(t, repoB, "recent work", "Local User", "local@example.com", time.Now().Add(-48*time.Hour))

	runGit(t, repoC, "config", "user.name", "Other User")
	runGit(t, repoC, "config", "user.email", "other@example.com")

	require.NoError(t, SetContext(m, wsHome, "repo-b", false))
	require.NoError(t, AddContext(m, wsHome, activeFilterToken, false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "repo-b,active", state.Raw)
	assert.Equal(t, []string{"repo-b", "repo-a"}, state.Resolved)
	assertScopeEntries(t, wsHome, "repo-a", "repo-b")
	assertWorkspaceFolders(t, filepath.Join(wsHome, m.Workspace), "~ workspace", "repo-b", "repo-a")
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

	initCheckout(t, filepath.Join(wsHome, "repos", "local-repo"))

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

func TestGetDefaultContextForMode_CollapsesResolvedWorktreesWhenDisabled(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
repos:
  repo:
  other:
`)
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(wsHome, contextFile), []byte(`
raw: repo,repo@repo-feature,other
resolved:
  - repo
  - repo@repo-feature
  - other
`), 0644))

	filter, ok := GetDefaultContextForMode(m, wsHome, true)
	require.True(t, ok)
	assert.Equal(t, "repo,repo@repo-feature,other", filter)

	filter, ok = GetDefaultContextForMode(m, wsHome, false)
	require.True(t, ok)
	assert.Equal(t, "repo,other", filter)
}

func TestSwapContext_RestoresPreviousRaw(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	require.NoError(t, SetContext(m, wsHome, "backend", false))
	require.NoError(t, SetContext(m, wsHome, "all", false))

	require.NoError(t, SwapContext(m, wsHome, false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "backend", state.Raw)
	require.NotNil(t, state.Previous)
	assert.Equal(t, "all", *state.Previous)
	assertScopeEntries(t, wsHome, "repo-a")
}

func TestSwapContext_NoPreviousAfterFirstSetErrors(t *testing.T) {
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

	require.NoError(t, SetContext(m, wsHome, "all", false))

	err = SwapContext(m, wsHome, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no previous context")
}

func TestRefreshContext_PreservesPrevious(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	require.NoError(t, SetContext(m, wsHome, "backend", false))
	require.NoError(t, SetContext(m, wsHome, "all", false))
	require.NoError(t, RefreshContext(m, wsHome, false))

	state := readStoredContext(t, wsHome)
	assert.Equal(t, "all", state.Raw)
	require.NotNil(t, state.Previous)
	assert.Equal(t, "backend", *state.Previous)
}

func TestSwapContext_ClearedPreviousSwapsToCleared(t *testing.T) {
	wsHome := t.TempDir()
	m, err := parseManifestYAML(`
root: repos
workspace: ws.code-workspace
remotes:
  default: git@example.com
groups:
  backend: [repo-a]
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-a"))
	initCheckout(t, filepath.Join(wsHome, "repos", "repo-b"))

	// Start cleared, then set a real context; previous should be "".
	require.NoError(t, SetContext(m, wsHome, "none", false))
	require.NoError(t, SetContext(m, wsHome, "backend", false))

	state := readStoredContext(t, wsHome)
	require.NotNil(t, state.Previous)
	assert.Equal(t, "", *state.Previous)

	// Swap back to cleared.
	require.NoError(t, SwapContext(m, wsHome, false))

	state = readStoredContext(t, wsHome)
	assert.Equal(t, "", state.Raw)
	require.NotNil(t, state.Previous)
	assert.Equal(t, "backend", *state.Previous)
}
