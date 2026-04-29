package command

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCompleteTopLevel(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
	assert.Equal(t, GroupSubcommands, result.Groups["init"])
	assert.NotEmpty(t, result.Descriptions["init"])
	assert.NotEmpty(t, result.Descriptions["install"])
}

func TestCompleteFiltersAreGroupedByCategory(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
groups:
  ai: [repo-a, repo-b, repo-c, repo-d]
repos:
  repo-a:
  repo-b:
  repo-c:
  repo-d:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"ll", ""}, 1)
	// Filter tokens, groups, repos, and flags should each carry their own
	// group label so zsh can _describe them separately.
	assert.Equal(t, GroupFilters, result.Groups["all"])
	assert.Equal(t, GroupGroups, result.Groups["ai"])
	assert.Equal(t, GroupRepos, result.Groups["repo-a"])
	assert.Equal(t, GroupFlags, result.Groups["--branches"])
	// Group description summarises members.
	assert.Contains(t, result.Descriptions["ai"], "repo-a")
	assert.Contains(t, result.Descriptions["ai"], "+1 more")
}

func TestCompletionOutput_GroupedFormat(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	lines := CompletionOutput(m, []string{"ll", ""}, 1)
	require.NotEmpty(t, lines)
	for _, line := range lines {
		// Each line must be 3 tab-separated fields.
		fields := strings.Split(line, "\t")
		assert.Len(t, fields, 3, "line should have 3 fields: %q", line)
	}
}

func TestCompletionOutput_FallbackSentinelStillSingleLine(t *testing.T) {
	// "ws git " — single sentinel line, either fallback or delegate form.
	lines := CompletionOutput(nil, []string{"git", ""}, 1)
	require.Len(t, lines, 1)
	assert.True(t,
		lines[0] == CompletionCommandFallbackSentinel ||
			strings.HasPrefix(lines[0], CompletionCommandFallbackSentinel+":"),
		"expected sentinel, got %q", lines[0])
	assert.NotContains(t, lines[0], "\t", "sentinel must stay single-field")
}

func TestCompleteTopLevelMarksFlagsAndCommands(t *testing.T) {
	result := Complete(nil, []string{""}, 0)
	assert.Equal(t, GroupFlags, result.Groups["-w"])
	assert.Equal(t, GroupSubcommands, result.Groups["ll"])
	// Commands lift their summary description.
	assert.NotEmpty(t, result.Descriptions["ll"])
}

func TestCompleteUpgradeSuggestsCheckFlag(t *testing.T) {
	result := Complete(nil, []string{"upgrade", ""}, 1)
	assert.Equal(t, []string{"--check"}, result.Values)
}

func TestCompleteUpgradeNoFurtherArgs(t *testing.T) {
	result := Complete(nil, []string{"upgrade", "--check", ""}, 2)
	assert.Empty(t, result.Values)
}

func TestCompleteRemotesSuggestsSync(t *testing.T) {
	result := Complete(nil, []string{"remotes", ""}, 1)
	assert.Equal(t, []string{"sync"}, result.Values)
}

func TestCompleteRemotesSyncSuggestsFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"remotes", "sync", ""}, 2)
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "all")
}

func TestCompleteFetchSuggestsRemoteFlagAndFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
groups:
  ai: [repo-a]
repos:
  repo-a:
    remotes:
      upstream: git@github.com:upstream/repo-a.git
`)
	require.NoError(t, err)

	result := Complete(m, []string{"fetch", ""}, 1)
	assert.Contains(t, result.Values, "--remote")
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
}

func TestCompleteFetchAfterRemoteSuggestsRemoteNames(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
repos:
  repo-a:
    remotes:
      upstream: git@github.com:upstream/repo-a.git
`)
	require.NoError(t, err)

	result := Complete(m, []string{"fetch", "--remote", ""}, 2)
	assert.Contains(t, result.Values, "origin")
	assert.Contains(t, result.Values, "upstream")
}

func TestCompleteRepairRefspecsSuggestsFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
groups:
  ai: [repo-a]
repos:
  repo-a:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"repair-refspecs", ""}, 1)
	assert.Contains(t, result.Values, "ai")
	assert.Contains(t, result.Values, "repo-a")
}

func TestCompleteLLIncludesWorktreesFlagAndFilters(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
