package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
