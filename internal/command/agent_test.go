package command

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchSessionRepo_ExactMatch(t *testing.T) {
	index := map[string]string{
		"/home/user/ws":          "(root)",
		"/home/user/repos/api":   "api",
		"/home/user/repos/web":   "web",
	}

	name, ok := matchSessionRepo("/home/user/repos/api", index)
	assert.True(t, ok)
	assert.Equal(t, "api", name)
}

func TestMatchSessionRepo_SubdirectoryMatch(t *testing.T) {
	index := map[string]string{
		"/home/user/ws":          "(root)",
		"/home/user/repos/api":   "api",
	}

	name, ok := matchSessionRepo("/home/user/repos/api/cmd/server", index)
	assert.True(t, ok)
	assert.Equal(t, "api", name)
}

func TestMatchSessionRepo_RootMatch(t *testing.T) {
	index := map[string]string{
		"/home/user/ws":          "(root)",
		"/home/user/repos/api":   "api",
	}

	name, ok := matchSessionRepo("/home/user/ws", index)
	assert.True(t, ok)
	assert.Equal(t, "(root)", name)
}

func TestMatchSessionRepo_LongestPrefixWins(t *testing.T) {
	index := map[string]string{
		"/home/user/ws":            "(root)",
		"/home/user/ws/repos":      "repos-meta",
		"/home/user/ws/repos/api":  "api",
	}

	name, ok := matchSessionRepo("/home/user/ws/repos/api/internal", index)
	assert.True(t, ok)
	assert.Equal(t, "api", name)
}

func TestMatchSessionRepo_NoMatch(t *testing.T) {
	index := map[string]string{
		"/home/user/ws":          "(root)",
		"/home/user/repos/api":   "api",
	}

	_, ok := matchSessionRepo("/other/path", index)
	assert.False(t, ok)
}

func TestMatchSessionRepo_NearMiss(t *testing.T) {
	// "/home/user/repos/api-v2" should not match "/home/user/repos/api"
	index := map[string]string{
		"/home/user/repos/api": "api",
	}

	_, ok := matchSessionRepo("/home/user/repos/api-v2", index)
	assert.False(t, ok)
}

func TestFormatTimeAgo(t *testing.T) {
	now := time.Now()

	tests := []struct {
		delta    time.Duration
		expected string
	}{
		{30 * time.Second, "30s ago"},
		{5 * time.Minute, "5m ago"},
		{3 * time.Hour, "3h ago"},
		{2 * 24 * time.Hour, "2d ago"},
		{2 * 7 * 24 * time.Hour, "2w ago"},
		{60 * 24 * time.Hour, "2mo ago"},
	}

	for _, tt := range tests {
		t.Run(tt.expected, func(t *testing.T) {
			result := formatTimeAgo(now, now.Add(-tt.delta))
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestFormatTimeAgo_Zero(t *testing.T) {
	assert.Equal(t, "", formatTimeAgo(time.Now(), time.Time{}))
}

func TestTruncateText(t *testing.T) {
	assert.Equal(t, "hello", truncateText("hello", 10))
	assert.Equal(t, "hello w...", truncateText("hello world", 10))
	assert.Equal(t, "", truncateText("hello", 0))
	assert.Equal(t, "hel", truncateText("hello", 3))
}

func TestTruncateText_Newlines(t *testing.T) {
	assert.Equal(t, "line one line two", truncateText("line one\nline two", 40))
}

func TestShellJoin(t *testing.T) {
	assert.Equal(t, "--resume abc123", shellJoin([]string{"--resume", "abc123"}))
	assert.Equal(t, "'hello world'", shellJoin([]string{"hello world"}))
	assert.Equal(t, "'it'\\''s'", shellJoin([]string{"it's"}))
}

func TestResolveSessionRef_ByIndex(t *testing.T) {
	sessions := []AgentSession{
		{SessionID: "aaa", Repo: "api"},
		{SessionID: "bbb", Repo: "web"},
	}

	s, err := resolveSessionRef(sessions, "1")
	require.NoError(t, err)
	assert.Equal(t, "aaa", s.SessionID)

	s, err = resolveSessionRef(sessions, "2")
	require.NoError(t, err)
	assert.Equal(t, "bbb", s.SessionID)
}

func TestResolveSessionRef_IndexOutOfRange(t *testing.T) {
	sessions := []AgentSession{{SessionID: "aaa"}}

	_, err := resolveSessionRef(sessions, "0")
	assert.Error(t, err)

	_, err = resolveSessionRef(sessions, "2")
	assert.Error(t, err)
}

func TestResolveSessionRef_ByPartialID(t *testing.T) {
	sessions := []AgentSession{
		{SessionID: "abc123-def"},
		{SessionID: "xyz789-ghi"},
	}

	s, err := resolveSessionRef(sessions, "abc")
	require.NoError(t, err)
	assert.Equal(t, "abc123-def", s.SessionID)
}

func TestResolveSessionRef_AmbiguousID(t *testing.T) {
	sessions := []AgentSession{
		{SessionID: "abc123"},
		{SessionID: "abc456"},
	}

	_, err := resolveSessionRef(sessions, "abc")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ambiguous")
}

func TestResolveSessionRef_NoMatch(t *testing.T) {
	sessions := []AgentSession{{SessionID: "abc123"}}

	_, err := resolveSessionRef(sessions, "xyz")
	assert.Error(t, err)
}

func TestResolveAgentCmd_FromManifest(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
agents:
  claude: IS_SANDBOX=1 claude --dangerously-skip-permissions
  codex: codex --yolo
repos:
  repo-a:
`)
	require.NoError(t, err)

	assert.Equal(t, "IS_SANDBOX=1 claude --dangerously-skip-permissions", resolveAgentCmd(m, "claude"))
	assert.Equal(t, "codex --yolo", resolveAgentCmd(m, "codex"))
}

func TestResolveAgentCmd_FallbackToBinaryName(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
`)
	require.NoError(t, err)

	assert.Equal(t, "claude", resolveAgentCmd(m, "claude"))
	assert.Equal(t, "codex", resolveAgentCmd(m, "codex"))
}

func TestResolveAgentCmd_DefaultFromManifest(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
agents:
  default: cc
  cc: IS_SANDBOX=1 claude --skip
repos:
  repo-a:
`)
	require.NoError(t, err)

	// Empty name resolves to default → cc → the configured command
	cmd := resolveAgentCmd(m, "")
	assert.Equal(t, "IS_SANDBOX=1 claude --skip", cmd)
}

func TestAgentResumeCmd_Claude(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
agents:
  claude: IS_SANDBOX=1 claude --skip
repos:
  repo-a:
`)
	require.NoError(t, err)

	s := AgentSession{Agent: agentClaude, SessionID: "abc-123"}
	cmd := agentResumeCmd(m, s)
	assert.Equal(t, "IS_SANDBOX=1 claude --skip --resume abc-123", cmd)
}

func TestAgentResumeCmd_Codex(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
agents:
  codex: codex --yolo
repos:
  repo-a:
`)
	require.NoError(t, err)

	s := AgentSession{Agent: agentCodex, SessionID: "xyz-789"}
	cmd := agentResumeCmd(m, s)
	assert.Equal(t, "codex --yolo resume xyz-789", cmd)
}

func TestFilterSessionsByRepo(t *testing.T) {
	sessions := []AgentSession{
		{SessionID: "a", Repo: "(root)"},
		{SessionID: "b", Repo: "api"},
		{SessionID: "c", Repo: "(root)"},
		{SessionID: "d", Repo: "web"},
	}

	result := filterSessionsByRepo(sessions, "(root)")
	assert.Len(t, result, 2)
	assert.Equal(t, "a", result[0].SessionID)
	assert.Equal(t, "c", result[1].SessionID)
}

func TestExternalRepoLabel(t *testing.T) {
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Home directory itself
	assert.Equal(t, "~", externalRepoLabel(home))

	// Direct child of home (1 component)
	assert.Equal(t, "~/projects", externalRepoLabel(filepath.Join(home, "projects")))

	// 2 components under home
	assert.Equal(t, "~/code/thing", externalRepoLabel(filepath.Join(home, "code", "thing")))

	// 3+ components collapse to last two
	assert.Equal(t, ".../deep/thing", externalRepoLabel(filepath.Join(home, "a", "b", "deep", "thing")))

	// Outside home — uses basename
	assert.Equal(t, "tmp", externalRepoLabel("/tmp"))
	assert.Equal(t, "project", externalRepoLabel("/opt/project"))
}

func TestBuildPathIndex_RootFilter(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	index := buildPathIndex(m, "/tmp/ws", agentFilterRoot, false)
	assert.Equal(t, map[string]string{"/tmp/ws": "(root)"}, index)
}

func TestReconcileClaudePermissionFlag(t *testing.T) {
	// Config has flag, session needs it → keep
	assert.Equal(t,
		"IS_SANDBOX=1 claude --dangerously-skip-permissions",
		reconcileClaudePermissionFlag("IS_SANDBOX=1 claude --dangerously-skip-permissions", true))

	// Config has flag, session doesn't need it → remove
	assert.Equal(t,
		"IS_SANDBOX=1 claude",
		reconcileClaudePermissionFlag("IS_SANDBOX=1 claude --dangerously-skip-permissions", false))

	// Config doesn't have flag, session needs it → add
	assert.Equal(t,
		"claude --dangerously-skip-permissions",
		reconcileClaudePermissionFlag("claude", true))

	// Config doesn't have flag, session doesn't need it → leave alone
	assert.Equal(t,
		"claude",
		reconcileClaudePermissionFlag("claude", false))
}

func TestAgentResumeCmd_ClaudeWithBypass(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
agents:
  claude: IS_SANDBOX=1 claude --dangerously-skip-permissions
repos:
  repo-a:
`)
	require.NoError(t, err)

	// Session WITH bypass → flag preserved
	s := AgentSession{Agent: agentClaude, SessionID: "abc-123", BypassPermissions: true}
	cmd := agentResumeCmd(m, s)
	assert.Equal(t, "IS_SANDBOX=1 claude --dangerously-skip-permissions --resume abc-123", cmd)

	// Session WITHOUT bypass → flag removed
	s.BypassPermissions = false
	cmd = agentResumeCmd(m, s)
	assert.Equal(t, "IS_SANDBOX=1 claude --resume abc-123", cmd)
}

func TestCompleteAgentTopLevel(t *testing.T) {
	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`)
	require.NoError(t, err)

	result := Complete(m, []string{"agent", ""}, 1)
	assert.Contains(t, result.Values, "ls")
	assert.Contains(t, result.Values, "resume")
	assert.Contains(t, result.Values, "--agent")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "repo-b")
}

func TestCompleteAgentLsIncludesFilters(t *testing.T) {
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

	result := Complete(m, []string{"agent", "ls", ""}, 2)
	assert.Contains(t, result.Values, "backend")
	assert.Contains(t, result.Values, "repo-a")
	assert.Contains(t, result.Values, "--all")
	assert.Contains(t, result.Values, "-n")
}
