package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestShellInitDelegatesCompletionToWrappedCommand(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not installed")
	}
	if _, err := exec.LookPath("go"); err != nil {
		t.Skip("go not installed")
	}

	wd, err := os.Getwd()
	require.NoError(t, err)

	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ws")
	build := exec.Command("go", "build", "-o", binPath, ".")
	build.Dir = wd
	build.Env = os.Environ()
	output, err := build.CombinedOutput()
	require.NoError(t, err, string(output))

	wsHome := filepath.Join(tmpDir, "workspace")
	require.NoError(t, os.MkdirAll(wsHome, 0755))
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
repos:
  mmdoc:
`), 0644))

	script := filepath.Join(tmpDir, "completion.sh")
	require.NoError(t, os.WriteFile(script, []byte(strings.TrimSpace(`
set -euo pipefail
export WS_HOME="`+wsHome+`"
export PATH="`+tmpDir+`:$PATH"
source <(ws init)
_git_complete() {
  COMPREPLY=( $(compgen -W "branch blame bisect" -- "$2") )
}
complete -F _git_complete git
COMP_WORDS=(ws mmdoc git br)
COMP_CWORD=3
COMP_LINE="ws mmdoc git br"
COMP_POINT=${#COMP_LINE}
_ws_complete_bash
printf '%s\n' "${COMPREPLY[@]}"
`)+"\n"), 0755))

	run := exec.Command("bash", script)
	run.Env = os.Environ()
	out, err := run.CombinedOutput()
	require.NoError(t, err, string(out))
	assert.Contains(t, strings.Fields(string(out)), "branch")
}

func TestParseContextArgs_Show(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, "show", action)
	assert.Equal(t, "", filter)
	assert.False(t, worktrees.Set)
}

func TestParseContextArgs_Set(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs([]string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", action)
	assert.Equal(t, "backend", filter)
	assert.False(t, worktrees.Set)
}

func TestParseContextArgs_Add(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs([]string{"add", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "add", action)
	assert.Equal(t, "backend,repo-a", filter)
	assert.False(t, worktrees.Set)
}

func TestParseContextArgs_SetWithWorktreesFlag(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs([]string{"-t", "backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", action)
	assert.Equal(t, "backend", filter)
	assert.True(t, worktrees.Set)
	assert.True(t, worktrees.Value)
}

func TestParseContextArgs_AddWithWorktreesFlag(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs([]string{"add", "-t", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "add", action)
	assert.Equal(t, "backend,repo-a", filter)
	assert.True(t, worktrees.Set)
	assert.True(t, worktrees.Value)
}

func TestParseContextArgs_SetWithNoWorktreesFlag(t *testing.T) {
	action, filter, worktrees, err := parseContextArgs([]string{"--no-worktrees", "backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", action)
	assert.Equal(t, "backend", filter)
	assert.True(t, worktrees.Set)
	assert.False(t, worktrees.Value)
}

func TestParseContextArgs_AddRequiresFilter(t *testing.T) {
	_, _, _, err := parseContextArgs([]string{"add"})
	require.Error(t, err)
}

func TestParseContextArgs_RejectsUnknownFlag(t *testing.T) {
	_, _, _, err := parseContextArgs([]string{"--bogus"})
	require.Error(t, err)
}

func TestResolveCDTarget_InlineWorktree(t *testing.T) {
	active := activeRepoConfigs(t, `
remotes:
  default: git@example.com
repos:
  mmdoc:
`)

	name, selector, err := resolveCDTarget("mmdoc@mmdoc-uv-master", "", active)
	require.NoError(t, err)
	assert.Equal(t, "mmdoc", name)
	assert.Equal(t, "mmdoc-uv-master", selector)
}

func TestResolveCDTarget_RejectsMixedSelectorForms(t *testing.T) {
	active := activeRepoConfigs(t, `
remotes:
  default: git@example.com
repos:
  mmdoc:
`)

	_, _, err := resolveCDTarget("mmdoc@mmdoc-uv-master", "feature/other", active)
	require.Error(t, err)
}

func TestResolveCDTarget_ExactRepoWins(t *testing.T) {
	active := activeRepoConfigs(t, `
remotes:
  default: git@example.com
repos:
  mmdoc@docs:
`)

	name, selector, err := resolveCDTarget("mmdoc@docs", "", active)
	require.NoError(t, err)
	assert.Equal(t, "mmdoc@docs", name)
	assert.Equal(t, "", selector)
}

func activeRepoConfigs(t *testing.T, yaml string) map[string]manifest.RepoConfig {
	t.Helper()

	m, err := manifest.Parse([]byte(yaml))
	require.NoError(t, err)
	return m.ActiveRepos()
}
