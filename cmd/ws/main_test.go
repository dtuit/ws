package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dtuit/ws/internal/command"
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
  origin: git@example.com:org
repos:
  mmdoc:
`), 0644))

	script := filepath.Join(tmpDir, "completion.sh")
	require.NoError(t, os.WriteFile(script, []byte(strings.TrimSpace(`
set -euo pipefail
export WS_HOME="`+wsHome+`"
export PATH="`+tmpDir+`:$PATH"
source <(ws shell init)
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

func TestUsageTextIncludesSharedCommandHelp(t *testing.T) {
	text := command.UsageText()

	assert.Contains(t, text, "ll [filter]")
	assert.Contains(t, text, "cd [repo[@worktree]]")
	assert.Contains(t, text, "context [subcommand]")
	assert.Contains(t, text, "worktree [subcommand]")
	assert.Contains(t, text, "repos [--all]")
	assert.Contains(t, text, "(alias: ctx)")
	assert.Contains(t, text, "(alias: wt)")
	assert.Contains(t, text, "(alias: list)")
	assert.Contains(t, text, "shell init|install")
	assert.Contains(t, text, "Inspect:")
	assert.Contains(t, text, "Scope:")
	assert.Contains(t, text, "Tools:")
	assert.NotContains(t, text, "\n  help ")
	assert.NotContains(t, text, "\n  init ")
	assert.NotContains(t, text, "\n  version ")
}

func TestParseShellArgsInit(t *testing.T) {
	parsed, err := parseShellArgs([]string{"init"})
	require.NoError(t, err)
	assert.Equal(t, "init", parsed.Action)
}

func TestParseShellArgsInstall(t *testing.T) {
	parsed, err := parseShellArgs([]string{"install"})
	require.NoError(t, err)
	assert.Equal(t, "install", parsed.Action)
}

func TestParseShellArgsRejectsInvalidInput(t *testing.T) {
	_, err := parseShellArgs(nil)
	require.Error(t, err)

	_, err = parseShellArgs([]string{"bogus"})
	require.Error(t, err)

	_, err = parseShellArgs([]string{"install", "extra"})
	require.Error(t, err)
}

func TestParseContextArgs_Show(t *testing.T) {
	parsed, err := parseContextArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, "show", parsed.Action)
	assert.Equal(t, "", parsed.Filter)
	assert.False(t, parsed.WorktreesOverride.Set)
}

func TestParseContextArgs_Set(t *testing.T) {
	parsed, err := parseContextArgs([]string{"backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", parsed.Action)
	assert.Equal(t, "backend", parsed.Filter)
	assert.False(t, parsed.WorktreesOverride.Set)
}

func TestParseContextArgs_Add(t *testing.T) {
	parsed, err := parseContextArgs([]string{"add", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "add", parsed.Action)
	assert.Equal(t, "backend,repo-a", parsed.Filter)
	assert.False(t, parsed.WorktreesOverride.Set)
}

func TestParseContextArgs_SetWithWorktreesFlag(t *testing.T) {
	parsed, err := parseContextArgs([]string{"-t", "backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", parsed.Action)
	assert.Equal(t, "backend", parsed.Filter)
	assert.True(t, parsed.WorktreesOverride.Set)
	assert.True(t, parsed.WorktreesOverride.Value)
}

func TestParseContextArgs_Remove(t *testing.T) {
	parsed, err := parseContextArgs([]string{"remove", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "remove", parsed.Action)
	assert.Equal(t, "backend,repo-a", parsed.Filter)
	assert.False(t, parsed.WorktreesOverride.Set)
}

func TestParseContextArgs_RemoveWithWorktreesFlag(t *testing.T) {
	parsed, err := parseContextArgs([]string{"remove", "-t", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "remove", parsed.Action)
	assert.Equal(t, "backend,repo-a", parsed.Filter)
	assert.True(t, parsed.WorktreesOverride.Set)
	assert.True(t, parsed.WorktreesOverride.Value)
}

func TestParseContextArgs_AddWithWorktreesFlag(t *testing.T) {
	parsed, err := parseContextArgs([]string{"add", "-t", "backend", "repo-a"})
	require.NoError(t, err)
	assert.Equal(t, "add", parsed.Action)
	assert.Equal(t, "backend,repo-a", parsed.Filter)
	assert.True(t, parsed.WorktreesOverride.Set)
	assert.True(t, parsed.WorktreesOverride.Value)
}

func TestParseContextArgs_SetWithNoWorktreesFlag(t *testing.T) {
	parsed, err := parseContextArgs([]string{"--no-worktrees", "backend"})
	require.NoError(t, err)
	assert.Equal(t, "set", parsed.Action)
	assert.Equal(t, "backend", parsed.Filter)
	assert.True(t, parsed.WorktreesOverride.Set)
	assert.False(t, parsed.WorktreesOverride.Value)
}

func TestParseContextArgs_Save(t *testing.T) {
	parsed, err := parseContextArgs([]string{"save", "focus"})
	require.NoError(t, err)
	assert.Equal(t, "save", parsed.Action)
	assert.Equal(t, "focus", parsed.Group)
	assert.False(t, parsed.Local)
}

func TestParseContextArgs_SaveLocal(t *testing.T) {
	parsed, err := parseContextArgs([]string{"save", "--local", "focus"})
	require.NoError(t, err)
	assert.Equal(t, "save", parsed.Action)
	assert.Equal(t, "focus", parsed.Group)
	assert.True(t, parsed.Local)
}

func TestParseContextArgs_SaveRejectsWorktreesFlag(t *testing.T) {
	_, err := parseContextArgs([]string{"save", "-t", "focus"})
	require.Error(t, err)
}

func TestParseContextArgs_Refresh(t *testing.T) {
	parsed, err := parseContextArgs([]string{"refresh"})
	require.NoError(t, err)
	assert.Equal(t, "refresh", parsed.Action)
	assert.Equal(t, "", parsed.Filter)
	assert.False(t, parsed.Local)
}

func TestParseContextArgs_RefreshWithWorktreesFlag(t *testing.T) {
	parsed, err := parseContextArgs([]string{"refresh", "-t"})
	require.NoError(t, err)
	assert.Equal(t, "refresh", parsed.Action)
	assert.True(t, parsed.WorktreesOverride.Set)
	assert.True(t, parsed.WorktreesOverride.Value)
}

func TestParseContextArgs_RefreshRejectsExtraArgs(t *testing.T) {
	_, err := parseContextArgs([]string{"refresh", "backend"})
	require.Error(t, err)
}

func TestParseOptionalFilterArgRejectsExtraPositionals(t *testing.T) {
	_, err := parseOptionalFilterArg([]string{"backend", "repo-a"}, "", false, "ws ll [filter]")
	require.EqualError(t, err, "usage: ws ll [filter]")
}

func TestParseOptionalFilterArgFallsBackToDefault(t *testing.T) {
	filter, err := parseOptionalFilterArg(nil, "backend", true, "ws ll [filter]")
	require.NoError(t, err)
	assert.Equal(t, "backend", filter)
}

func TestParseCDArgsWorktreeFlag(t *testing.T) {
	name, selector, err := parseCDArgs([]string{"repo-a", "-t", "feature/auth"})
	require.NoError(t, err)
	assert.Equal(t, "repo-a", name)
	assert.Equal(t, "feature/auth", selector)
}

func TestParseContextArgs_RejectsLocalWithoutSave(t *testing.T) {
	_, err := parseContextArgs([]string{"set", "--local", "backend"})
	require.Error(t, err)
}

func TestParseContextArgs_AddRequiresFilter(t *testing.T) {
	_, err := parseContextArgs([]string{"add"})
	require.Error(t, err)
}

func TestParseContextArgs_RemoveRequiresFilter(t *testing.T) {
	_, err := parseContextArgs([]string{"remove"})
	require.Error(t, err)
}

func TestParseContextArgs_RejectsUnknownFlag(t *testing.T) {
	_, err := parseContextArgs([]string{"--bogus"})
	require.Error(t, err)
}

func TestResolveCDTarget_InlineWorktree(t *testing.T) {
	active := activeRepoConfigs(t, `
remotes:
  origin: git@example.com:org
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
  origin: git@example.com:org
repos:
  mmdoc:
`)

	_, _, err := resolveCDTarget("mmdoc@mmdoc-uv-master", "feature/other", active)
	require.Error(t, err)
}

func TestResolveCDTarget_ExactRepoWins(t *testing.T) {
	active := activeRepoConfigs(t, `
remotes:
  origin: git@example.com:org
repos:
  mmdoc@docs:
`)

	name, selector, err := resolveCDTarget("mmdoc@docs", "", active)
	require.NoError(t, err)
	assert.Equal(t, "mmdoc@docs", name)
	assert.Equal(t, "", selector)
}

func TestParseMuxArgs_DupNoArgs(t *testing.T) {
	parsed, err := parseMuxArgs([]string{"dup"})
	require.NoError(t, err)
	assert.Equal(t, "dup", parsed.Action)
	assert.Empty(t, parsed.WindowName)
}

func TestParseMuxArgs_DupWithWindow(t *testing.T) {
	parsed, err := parseMuxArgs([]string{"dup", "editor"})
	require.NoError(t, err)
	assert.Equal(t, "dup", parsed.Action)
	assert.Equal(t, "editor", parsed.WindowName)
}

func TestParseMuxArgs_DuplicateAlias(t *testing.T) {
	parsed, err := parseMuxArgs([]string{"duplicate", "shell"})
	require.NoError(t, err)
	assert.Equal(t, "dup", parsed.Action)
	assert.Equal(t, "shell", parsed.WindowName)
}

func TestParseMuxArgs_DupTooManyArgs(t *testing.T) {
	_, err := parseMuxArgs([]string{"dup", "editor", "extra"})
	require.Error(t, err)
}

func TestParseAgentArgs_DefaultStart(t *testing.T) {
	parsed, err := parseAgentArgs(nil)
	require.NoError(t, err)
	assert.Equal(t, "start", parsed.Action)
	assert.Empty(t, parsed.Repo)
	assert.Empty(t, parsed.Agent)
}

func TestParseAgentArgs_StartWithRepo(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"api-server"})
	require.NoError(t, err)
	assert.Equal(t, "start", parsed.Action)
	assert.Equal(t, "api-server", parsed.Repo)
}

func TestParseAgentArgs_StartWithAgentFlag(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"--agent", "codex", "api-server"})
	require.NoError(t, err)
	assert.Equal(t, "start", parsed.Action)
	assert.Equal(t, "codex", parsed.Agent)
	assert.Equal(t, "api-server", parsed.Repo)
}

func TestParseAgentArgs_StartWithPassthrough(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"api-server", "--", "--resume", "abc123"})
	require.NoError(t, err)
	assert.Equal(t, "start", parsed.Action)
	assert.Equal(t, "api-server", parsed.Repo)
	assert.Equal(t, []string{"--resume", "abc123"}, parsed.Passthrough)
}

func TestParseAgentArgs_LS(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls"})
	require.NoError(t, err)
	assert.Equal(t, "ls", parsed.Action)
	assert.Empty(t, parsed.Filter)
}

func TestParseAgentArgs_LSWithFilter(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls", "backend"})
	require.NoError(t, err)
	assert.Equal(t, "ls", parsed.Action)
	assert.Equal(t, "backend", parsed.Filter)
}

func TestParseAgentArgs_LSWithLimit(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls", "-n", "5"})
	require.NoError(t, err)
	assert.Equal(t, "ls", parsed.Action)
	assert.Equal(t, 5, parsed.Limit)
}

func TestParseAgentArgs_LSAll(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls", "--all"})
	require.NoError(t, err)
	assert.Equal(t, "ls", parsed.Action)
	assert.True(t, parsed.ShowAll)
}

func TestParseAgentArgs_LSLast(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls", "--last"})
	require.NoError(t, err)
	assert.True(t, parsed.ShowLast)
	assert.False(t, parsed.ShowRecap)

	parsed, err = parseAgentArgs([]string{"ls", "-l"})
	require.NoError(t, err)
	assert.True(t, parsed.ShowLast)
}

func TestParseAgentArgs_LSRecap(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"ls", "--recap"})
	require.NoError(t, err)
	assert.True(t, parsed.ShowRecap)
	assert.False(t, parsed.ShowLast)

	parsed, err = parseAgentArgs([]string{"ls", "-r"})
	require.NoError(t, err)
	assert.True(t, parsed.ShowRecap)
}

func TestParseAgentArgs_LSLastRecapMutuallyExclusive(t *testing.T) {
	_, err := parseAgentArgs([]string{"ls", "--last", "--recap"})
	require.Error(t, err)
}

func TestParseAgentArgs_Resume(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"resume", "3"})
	require.NoError(t, err)
	assert.Equal(t, "resume", parsed.Action)
	assert.Equal(t, "3", parsed.IndexOrID)
}

func TestParseAgentArgs_ResumeRequiresArg(t *testing.T) {
	_, err := parseAgentArgs([]string{"resume"})
	require.Error(t, err)
}

func TestParseAgentArgs_PassthroughOnly(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"--", "-r"})
	require.NoError(t, err)
	assert.Equal(t, "start", parsed.Action)
	assert.Empty(t, parsed.Repo)
	assert.Equal(t, []string{"-r"}, parsed.Passthrough)
}

func TestParseAgentArgs_SearchSimple(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"search", "ocr", "rewrite"})
	require.NoError(t, err)
	assert.Equal(t, "search", parsed.Action)
	assert.Equal(t, "ocr rewrite", parsed.Query)
	assert.False(t, parsed.External)
	assert.False(t, parsed.Verbose)
	assert.Equal(t, 0, parsed.Limit)
}

func TestParseAgentArgs_SearchFlags(t *testing.T) {
	parsed, err := parseAgentArgs([]string{"search", "--external", "-v", "-n", "5", "find me a session"})
	require.NoError(t, err)
	assert.Equal(t, "search", parsed.Action)
	assert.Equal(t, "find me a session", parsed.Query)
	assert.True(t, parsed.External)
	assert.True(t, parsed.Verbose)
	assert.Equal(t, 5, parsed.Limit)
}

func TestParseAgentArgs_SearchRequiresQuery(t *testing.T) {
	_, err := parseAgentArgs([]string{"search", "--external"})
	require.Error(t, err)
}

func TestParseAgentArgs_SearchUnknownFlag(t *testing.T) {
	_, err := parseAgentArgs([]string{"search", "--bogus", "query"})
	require.Error(t, err)
}

func TestParseAgentArgs_SearchInvalidLimit(t *testing.T) {
	_, err := parseAgentArgs([]string{"search", "-n", "0", "query"})
	require.Error(t, err)
}

func activeRepoConfigs(t *testing.T, yaml string) map[string]manifest.RepoConfig {
	t.Helper()

	if !strings.Contains(yaml, "\nroot:") && !strings.HasPrefix(strings.TrimSpace(yaml), "root:") {
		yaml = "root: repos\n" + yaml
	}
	if !strings.Contains(yaml, "\nremotes:") && !strings.HasPrefix(strings.TrimSpace(yaml), "remotes:") {
		yaml = "remotes:\n  origin: git@test:org\n" + yaml
	}
	m, err := manifest.Parse([]byte(yaml))
	require.NoError(t, err)
	return m.ActiveRepos()
}
