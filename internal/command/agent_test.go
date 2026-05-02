package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
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

func TestEnrichClaudeSessionMetadata_LatestNameWins(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	require.NoError(t, os.MkdirAll(sessionsDir, 0o755))

	// Two metadata files for the same session ID: the newer updatedAt
	// should supply the name, even when that name is set and the older
	// one was unnamed.
	oldFile := `{"pid":1,"sessionId":"sess-1","updatedAt":1000}`
	newFile := `{"pid":2,"sessionId":"sess-1","name":"hotfix","updatedAt":2000}`
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "1.json"), []byte(oldFile), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "2.json"), []byte(newFile), 0o644))

	// A separate session whose latest record cleared the name.
	clearedOld := `{"pid":3,"sessionId":"sess-2","name":"stale","updatedAt":500}`
	clearedNew := `{"pid":4,"sessionId":"sess-2","updatedAt":600}`
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "3.json"), []byte(clearedOld), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "4.json"), []byte(clearedNew), 0o644))

	sessions := []AgentSession{
		{SessionID: "sess-1"},
		{SessionID: "sess-2"},
		{SessionID: "missing"},
	}
	enrichClaudeSessionMetadata(sessions, home)

	assert.Equal(t, "hotfix", sessions[0].Name)
	assert.Equal(t, "", sessions[1].Name, "latest record with empty name should clear")
	assert.Equal(t, "", sessions[2].Name)
}

func TestEnrichClaudeSessionMetadata_IgnoresBadFiles(t *testing.T) {
	home := t.TempDir()
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	require.NoError(t, os.MkdirAll(sessionsDir, 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "garbage.json"), []byte("not json"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "empty-sid.json"),
		[]byte(`{"pid":0,"sessionId":"","name":"ignored"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sessionsDir, "good.json"),
		[]byte(fmt.Sprintf(`{"pid":0,"sessionId":"sess-x","name":"keep","updatedAt":1}`)), 0o644))

	sessions := []AgentSession{{SessionID: "sess-x"}}
	enrichClaudeSessionMetadata(sessions, home)
	assert.Equal(t, "keep", sessions[0].Name)
}

func TestCodexThreadName(t *testing.T) {
	cases := []struct {
		name             string
		title            string
		firstUserMessage string
		want             string
	}{
		{name: "renamed", title: "fix-bifrost", firstUserMessage: "Use a subagent to do X", want: "fix-bifrost"},
		{name: "title equals prompt", title: "do X", firstUserMessage: "do X", want: ""},
		{name: "title with surrounding whitespace equals prompt", title: "  do X  ", firstUserMessage: "do X", want: ""},
		{name: "empty title", title: "", firstUserMessage: "anything", want: ""},
		{name: "whitespace-only title", title: "   ", firstUserMessage: "anything", want: ""},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.want, codexThreadName(tc.title, tc.firstUserMessage))
		})
	}
}

func TestParseRenameArgs(t *testing.T) {
	cases := []struct {
		name      string
		content   string
		want      string
		wantFound bool
	}{
		{
			name:      "input event with name",
			content:   "<command-name>/rename</command-name>\n<command-message>rename</command-message>\n<command-args>fix-thing</command-args>",
			want:      "fix-thing",
			wantFound: true,
		},
		{
			name:      "input event with empty args (clear)",
			content:   "<command-name>/rename</command-name>\n<command-args></command-args>",
			want:      "",
			wantFound: true,
		},
		{
			name:      "stdout event has no command-args",
			content:   "<local-command-stdout>Session renamed to: foo</local-command-stdout>",
			want:      "",
			wantFound: false,
		},
		{
			name:      "different command",
			content:   "<command-name>/status</command-name>\n<command-args>x</command-args>",
			want:      "",
			wantFound: false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := parseRenameArgs(tc.content)
			assert.Equal(t, tc.wantFound, ok)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestReadClaudeNameFromTranscript(t *testing.T) {
	claudeDir := t.TempDir()
	projectPath := "/home/u/repo"
	sessionID := "sess-1"
	dirName := "-home-u-repo"
	transcriptDir := filepath.Join(claudeDir, "projects", dirName)
	require.NoError(t, os.MkdirAll(transcriptDir, 0o755))

	// Two /rename events: the second (latest) wins.
	lines := []string{
		`{"type":"user","message":"hello","sessionId":"sess-1"}`,
		`{"type":"system","subtype":"local_command","content":"<command-name>/rename</command-name>\n<command-args>first-name</command-args>"}`,
		`{"type":"system","subtype":"local_command","content":"<local-command-stdout>Session renamed to: first-name</local-command-stdout>"}`,
		`{"type":"system","subtype":"local_command","content":"<command-name>/rename</command-name>\n<command-args>final-name</command-args>"}`,
	}
	jsonlPath := filepath.Join(transcriptDir, sessionID+".jsonl")
	require.NoError(t, os.WriteFile(jsonlPath, []byte(strings.Join(lines, "\n")), 0o644))

	name, ok := readClaudeNameFromTranscript(claudeDir, projectPath, sessionID)
	assert.True(t, ok)
	assert.Equal(t, "final-name", name)

	// Missing file: returns ("", false)
	name, ok = readClaudeNameFromTranscript(claudeDir, projectPath, "missing")
	assert.False(t, ok)
	assert.Equal(t, "", name)

	// Latest /rename clears the name.
	clearedID := "sess-2"
	clearedLines := []string{
		`{"type":"system","subtype":"local_command","content":"<command-name>/rename</command-name>\n<command-args>old-name</command-args>"}`,
		`{"type":"system","subtype":"local_command","content":"<command-name>/rename</command-name>\n<command-args></command-args>"}`,
	}
	require.NoError(t, os.WriteFile(filepath.Join(transcriptDir, clearedID+".jsonl"),
		[]byte(strings.Join(clearedLines, "\n")), 0o644))
	name, ok = readClaudeNameFromTranscript(claudeDir, projectPath, clearedID)
	assert.True(t, ok, "explicit clear should still report found")
	assert.Equal(t, "", name)
}

func TestSelectCompactText(t *testing.T) {
	full := AgentSession{Prompt: "first", LastPrompt: "last", Summary: "recap"}
	onlyFirst := AgentSession{Prompt: "first"}
	firstAndLast := AgentSession{Prompt: "first", LastPrompt: "last"}

	// Default: first
	assert.Equal(t, "first", selectCompactText(full, AgentListMode{}))

	// ShowLast: last, fall back to first when last is empty
	assert.Equal(t, "last", selectCompactText(full, AgentListMode{ShowLast: true}))
	assert.Equal(t, "first", selectCompactText(onlyFirst, AgentListMode{ShowLast: true}))

	// ShowRecap: recap, fall back to last, then first
	assert.Equal(t, "recap", selectCompactText(full, AgentListMode{ShowRecap: true}))
	assert.Equal(t, "last", selectCompactText(firstAndLast, AgentListMode{ShowRecap: true}))
	assert.Equal(t, "first", selectCompactText(onlyFirst, AgentListMode{ShowRecap: true}))
}

func TestAgentListModeNeedsEnrichment(t *testing.T) {
	assert.False(t, AgentListMode{}.needsEnrichment())
	assert.True(t, AgentListMode{Verbose: true}.needsEnrichment())
	assert.True(t, AgentListMode{ShowLast: true}.needsEnrichment())
	assert.True(t, AgentListMode{ShowRecap: true}.needsEnrichment())
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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
  origin: git@example.com:org
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

func TestAgentPinsRoundTrip(t *testing.T) {
	dir := t.TempDir()

	pins, err := loadAgentPins(dir)
	require.NoError(t, err)
	assert.Empty(t, pins)

	added, err := addAgentPin(dir, "abc-123")
	require.NoError(t, err)
	assert.True(t, added)

	added, err = addAgentPin(dir, "abc-123")
	require.NoError(t, err)
	assert.False(t, added, "re-add should be a no-op")

	pins, err = loadAgentPins(dir)
	require.NoError(t, err)
	assert.True(t, pins["abc-123"])

	removed, err := removeAgentPin(dir, "abc-123")
	require.NoError(t, err)
	assert.True(t, removed)

	removed, err = removeAgentPin(dir, "abc-123")
	require.NoError(t, err)
	assert.False(t, removed)

	// File should be cleaned up when no pins remain.
	_, err = os.Stat(filepath.Join(dir, wsStateDir, agentPinsStateFile))
	assert.True(t, os.IsNotExist(err), "pins file should be removed when empty")
	_, err = os.Stat(filepath.Join(dir, legacyAgentPinsFile))
	assert.True(t, os.IsNotExist(err), "legacy pins file should not exist")
}

func TestAgentPinsMigratesLegacyFileOnWrite(t *testing.T) {
	dir := t.TempDir()

	// Seed a legacy flat pins file from a prior ws version.
	legacyPath := filepath.Join(dir, legacyAgentPinsFile)
	require.NoError(t, os.WriteFile(legacyPath, []byte("pins:\n  - legacy-id\n"), 0644))

	// Read path should fall back to the legacy file.
	pins, err := loadAgentPins(dir)
	require.NoError(t, err)
	assert.True(t, pins["legacy-id"])

	// Adding a new pin writes to the nested path and removes the legacy file.
	added, err := addAgentPin(dir, "new-id")
	require.NoError(t, err)
	assert.True(t, added)

	_, err = os.Stat(legacyPath)
	assert.True(t, os.IsNotExist(err), "legacy flat file should be migrated away")

	newPath := filepath.Join(dir, wsStateDir, agentPinsStateFile)
	_, err = os.Stat(newPath)
	require.NoError(t, err, "new nested pins file should exist after write")

	// Re-reading should see both the migrated legacy pin and the new one.
	pins, err = loadAgentPins(dir)
	require.NoError(t, err)
	assert.True(t, pins["legacy-id"])
	assert.True(t, pins["new-id"])
}

func TestApplyAgentPinsSortsPinnedFirst(t *testing.T) {
	now := time.Now()
	sessions := []AgentSession{
		{SessionID: "fresh", LastActive: now},
		{SessionID: "older", LastActive: now.Add(-time.Hour)},
		{SessionID: "oldest", LastActive: now.Add(-24 * time.Hour)},
	}

	result := applyAgentPins(sessions, map[string]bool{"oldest": true})

	assert.Equal(t, "oldest", result[0].SessionID)
	assert.True(t, result[0].Pinned)
	assert.Equal(t, "fresh", result[1].SessionID)
	assert.False(t, result[1].Pinned)
}

func TestTruncatePreservingPinsKeepsPins(t *testing.T) {
	sessions := []AgentSession{
		{SessionID: "p1", Pinned: true},
		{SessionID: "a"},
		{SessionID: "b"},
		{SessionID: "c"},
		{SessionID: "p2", Pinned: true},
	}

	// Limit of 2 should keep both pins and drop every unpinned entry.
	result := truncatePreservingPins(sessions, 2)
	require.Len(t, result, 2)
	assert.Equal(t, "p1", result[0].SessionID)
	assert.Equal(t, "p2", result[1].SessionID)

	// Limit of 3 keeps pins plus one unpinned (first in input order).
	result = truncatePreservingPins(sessions, 3)
	require.Len(t, result, 3)
	assert.Equal(t, "p1", result[0].SessionID)
	assert.Equal(t, "p2", result[1].SessionID)
	assert.Equal(t, "a", result[2].SessionID)
}
