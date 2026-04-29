package command

import (
	"strings"
	"testing"
	"time"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseSearchResponse_Raw(t *testing.T) {
	out := `{"matches":[{"id":"abc","reason":"OCR work"},{"id":"def","reason":"PR review"}]}`
	matches, err := parseSearchResponse(out)
	require.NoError(t, err)
	require.Len(t, matches, 2)
	assert.Equal(t, "abc", matches[0].ID)
	assert.Equal(t, "OCR work", matches[0].Reason)
	assert.Equal(t, "def", matches[1].ID)
}

func TestParseSearchResponse_FencedAndPreamble(t *testing.T) {
	out := "Here are the matches:\n```json\n{\"matches\":[{\"id\":\"xyz\",\"reason\":\"matches query\"}]}\n```\nDone."
	matches, err := parseSearchResponse(out)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "xyz", matches[0].ID)
}

func TestParseSearchResponse_StringContainingBraces(t *testing.T) {
	// A reason that contains a literal "}" must not terminate the object early.
	out := `prefix {"matches":[{"id":"abc","reason":"contains } in text"}]}`
	matches, err := parseSearchResponse(out)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "contains } in text", matches[0].Reason)
}

func TestParseSearchResponse_EmptyMatches(t *testing.T) {
	matches, err := parseSearchResponse(`{"matches":[]}`)
	require.NoError(t, err)
	assert.Empty(t, matches)
}

func TestParseSearchResponse_NoJSON(t *testing.T) {
	_, err := parseSearchResponse("Sorry, I cannot help.")
	require.Error(t, err)
}

func TestParseSearchResponse_UnterminatedJSON(t *testing.T) {
	_, err := parseSearchResponse(`{"matches":[{"id":"abc"`)
	require.Error(t, err)
}

func TestExtractJSONObject_HandlesEscapedQuote(t *testing.T) {
	// An escaped quote inside a string should not close the string.
	in := `noise {"k":"a\"b}","x":1} trailing`
	out, err := extractJSONObject(in)
	require.NoError(t, err)
	assert.Equal(t, `{"k":"a\"b}","x":1}`, out)
}

func TestBuildSearchCatalog_VerboseTogglesLastPrompt(t *testing.T) {
	sessions := []AgentSession{
		{
			Agent:      agentClaude,
			SessionID:  "abc",
			Repo:       "api",
			LastActive: time.Now().Add(-1 * time.Hour),
			Prompt:     "first",
			LastPrompt: "last",
			Summary:    "recap",
			Name:       "named",
		},
	}

	compact := buildSearchCatalog(sessions, false)
	require.Len(t, compact, 1)
	assert.Equal(t, "abc", compact[0].ID)
	assert.Equal(t, "first", compact[0].FirstPrompt)
	assert.Equal(t, "recap", compact[0].Recap)
	assert.Equal(t, "named", compact[0].Name)
	assert.Empty(t, compact[0].LastPrompt, "compact mode must not include last_prompt")

	verbose := buildSearchCatalog(sessions, true)
	assert.Equal(t, "last", verbose[0].LastPrompt)
}

func TestResolveSearcherCmd_Default(t *testing.T) {
	t.Setenv(agentSearchEnvCmd, "")
	got := resolveSearcherCmd(&manifest.Manifest{})
	assert.Equal(t, agentSearchDefaultCmd, got)
}

func TestResolveSearcherCmd_Manifest(t *testing.T) {
	t.Setenv(agentSearchEnvCmd, "")
	m := &manifest.Manifest{Agents: map[string]string{"search": "codex exec"}}
	assert.Equal(t, "codex exec", resolveSearcherCmd(m))
}

func TestResolveSearcherCmd_EnvWins(t *testing.T) {
	t.Setenv(agentSearchEnvCmd, "claude --append-system-prompt foo -p")
	m := &manifest.Manifest{Agents: map[string]string{"search": "codex exec"}}
	assert.Equal(t, "claude --append-system-prompt foo -p", resolveSearcherCmd(m))
}

func TestLookupSessionForMatch_PrefixFallback(t *testing.T) {
	by := map[string]AgentSession{
		"abc12345": {SessionID: "abc12345"},
		"def67890": {SessionID: "def67890"},
	}
	s, ok := lookupSessionForMatch(by, "abc")
	assert.True(t, ok)
	assert.Equal(t, "abc12345", s.SessionID)

	_, ok = lookupSessionForMatch(by, "missing")
	assert.False(t, ok)
}

func TestLookupSessionForMatch_AmbiguousPrefixFails(t *testing.T) {
	by := map[string]AgentSession{
		"abc1": {SessionID: "abc1"},
		"abc2": {SessionID: "abc2"},
	}
	_, ok := lookupSessionForMatch(by, "abc")
	assert.False(t, ok, "ambiguous prefix should not resolve")
}

func TestBuildSearchPrompt_ContainsQueryAndCatalog(t *testing.T) {
	sessions := []AgentSession{
		{Agent: agentClaude, SessionID: "abc", Repo: "api", LastActive: time.Now(), Prompt: "Refactor OCR"},
	}
	prompt := buildSearchPrompt("OCR rewrite", sessions, false, 5)
	assert.Contains(t, prompt, "OCR rewrite")
	assert.Contains(t, prompt, "Refactor OCR")
	assert.Contains(t, prompt, `"id":"abc"`)
	assert.Contains(t, prompt, "max 5 matches")
	assert.NotContains(t, strings.ToLower(prompt), "last_prompt:", "compact mode must not document last_prompt")
}

func TestBuildSearchPrompt_VerboseDocumentsLastPrompt(t *testing.T) {
	sessions := []AgentSession{
		{Agent: agentClaude, SessionID: "abc", Repo: "api", LastActive: time.Now(), Prompt: "p", LastPrompt: "lp"},
	}
	prompt := buildSearchPrompt("q", sessions, true, 10)
	assert.Contains(t, prompt, "last_prompt:")
}
