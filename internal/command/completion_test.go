package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	assert.Contains(t, result.Values, "shell")
	assert.Contains(t, result.Values, "ctx")
	assert.Contains(t, result.Values, "dirs")
	assert.Contains(t, result.Values, "backend")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "active")
	assert.Contains(t, result.Values, "dirty")
	assert.Contains(t, result.Values, "mine:1d")
	assert.Contains(t, result.Values, "--workspace")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
	assert.NotContains(t, result.Values, "init")
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

func TestCompleteCDIncludesWorktreeSelectorFlags(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"cd", "repo-a", ""}, 2)
	assert.Contains(t, result.Values, "--worktree")
	assert.Contains(t, result.Values, "-t")
}

func TestCompleteSetupIncludesFilters(t *testing.T) {
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
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "all")
}

func TestCompleteShellIncludesSubcommands(t *testing.T) {
	result := Complete(nil, []string{"shell", ""}, 1)
	assert.Contains(t, result.Values, "init")
	assert.Contains(t, result.Values, "install")
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
	assert.Contains(t, result.Values, "-b")
	assert.Contains(t, result.Values, "--branches")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "ai")
}

func TestCompleteReposIncludesWorktreesFlagAndShowAll(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"repos", ""}, 1)
	assert.Contains(t, result.Values, "--all")
	assert.Contains(t, result.Values, "-a")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "--worktrees")
}

func TestCompleteDirsIncludesFilters(t *testing.T) {
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

	result := Complete(m, []string{"dirs", ""}, 1)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.Contains(t, result.Values, "backend")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "all")
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
	assert.Contains(t, result.Values, "remove")
	assert.Contains(t, result.Values, "refresh")
	assert.Contains(t, result.Values, "active")
	assert.Contains(t, result.Values, "dirty")
	assert.Contains(t, result.Values, "none")
	assert.Contains(t, result.Values, "reset")
}

func TestCompleteContextAliasIncludesContextSuggestions(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"ctx", ""}, 1)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "add")
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

func TestCompleteContextRefreshDoesNotSuggestFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", "refresh", ""}, 2)
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--worktrees")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.NotContains(t, result.Values, "ai")
	assert.NotContains(t, result.Values, "repo-a")
	assert.False(t, result.FallbackCommands)
}

func TestCompleteContextRemoveSuggestsFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"context", "remove", ""}, 2)
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "-t")
	assert.Contains(t, result.Values, "--no-worktrees")
	assert.NotContains(t, result.Values, "reset")
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
