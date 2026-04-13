package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveContextGroup_WritesManifest(t *testing.T) {
	wsHome := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
groups:
  existing: [repo-c]
repos:
  repo-a:
  repo-b:
  repo-c:
`), 0644))

	m, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)
	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "repo-a,repo-b", []manifest.RepoInfo{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}, nil))

	require.NoError(t, SaveContextGroup(m, wsHome, "focus", false))

	saved, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"repo-a", "repo-b"}, saved.Groups["focus"])
	assert.Equal(t, []string{"repo-c"}, saved.Groups["existing"])
}

func TestSaveContextGroup_LocalCreatesManifestLocal(t *testing.T) {
	wsHome := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`), 0644))

	m, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)
	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "repo-a,repo-b", []manifest.RepoInfo{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}, nil))

	require.NoError(t, SaveContextGroup(m, wsHome, "focus", true))

	merged, err := manifest.LoadWithLocal(wsHome)
	require.NoError(t, err)
	assert.Equal(t, []string{"repo-a", "repo-b"}, merged.Groups["focus"])
}

func TestSaveContextGroup_PreservesManifestWhitespace(t *testing.T) {
	wsHome := t.TempDir()
	manifestPath := filepath.Join(wsHome, "manifest.yml")
	original := `# Header

root: repos

groups:
  existing: [repo-c]

# Separator comment
repos:
  repo-a:
  repo-b:
  repo-c:
`
	require.NoError(t, os.WriteFile(manifestPath, []byte(original), 0644))

	m, err := manifest.Load(manifestPath)
	require.NoError(t, err)
	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "repo-a,repo-b", []manifest.RepoInfo{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}, nil))

	require.NoError(t, SaveContextGroup(m, wsHome, "focus", false))

	data, err := os.ReadFile(manifestPath)
	require.NoError(t, err)
	assert.Equal(t, `# Header

root: repos

groups:
  existing: [repo-c]
  focus:
    - repo-a
    - repo-b

# Separator comment
repos:
  repo-a:
  repo-b:
  repo-c:
`, string(data))
}

func TestSaveContextGroup_CollapsesWorktreesToBaseRepos(t *testing.T) {
	wsHome := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
repos:
  repo:
`), 0644))

	m, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)
	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "repo@repo-feature,repo", []manifest.RepoInfo{
		{Name: "repo@repo-feature", Worktree: "repo-feature"},
		{Name: "repo"},
	}, nil))

	require.NoError(t, SaveContextGroup(m, wsHome, "focus", false))

	saved, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)
	assert.Equal(t, []string{"repo"}, saved.Groups["focus"])
}

func TestSaveContextGroup_RejectsLocalOnlyReposInSharedManifest(t *testing.T) {
	wsHome := t.TempDir()
	m := loadManifestWithLocal(t, wsHome, `
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
`, `
repos:
  local-repo:
`)
	require.NoError(t, writeContextState(filepath.Join(wsHome, contextFile), "local-repo", []manifest.RepoInfo{
		{Name: "local-repo"},
	}, nil))

	err := SaveContextGroup(m, wsHome, "focus", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "local-repo")
	assert.Contains(t, err.Error(), "--local")
	assert.Contains(t, err.Error(), "manifest.local.yml")
}

func TestSaveContextGroup_RejectsWhenNoContextIsSet(t *testing.T) {
	wsHome := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
`), 0644))

	m, err := manifest.Load(filepath.Join(wsHome, "manifest.yml"))
	require.NoError(t, err)

	err = SaveContextGroup(m, wsHome, "focus", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no context set")
}

func TestValidateContextGroupName_RejectsReservedActivityFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	for _, group := range []string{activeFilterToken, dirtyFilterToken, mineFilterToken, "mine:1d", "active:1d"} {
		err = validateContextGroupName(m, group)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "reserved filter name")
	}
}

func TestCompleteContextSuggestsSave(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", ""}, 1)
	assert.Contains(t, result.Values, "save")
}

func TestCompleteContextSaveSuggestsLocal(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", "save", ""}, 2)
	assert.Equal(t, []string{"--local"}, result.Values)
}
