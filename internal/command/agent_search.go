package command

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
	xterm "golang.org/x/term"
)

// AgentSearchOptions controls the search command.
type AgentSearchOptions struct {
	External   bool // include sessions outside the workspace
	Verbose    bool // include last_prompt in the catalog (more tokens)
	CatalogMax int  // cap catalog at this many sessions (0 = default)
	MaxResults int  // cap results from the LLM (advisory, 0 = default)
}

const (
	agentSearchDefaultCatalog = 200
	agentSearchDefaultResults = 10
	agentSearchEnvCmd         = "WS_AGENT_SEARCH_CMD"
	agentSearchManifestKey    = "search"
	agentSearchDefaultCmd     = "claude -p"
)

// catalogEntry is one row in the JSON catalog sent to the LLM.
type catalogEntry struct {
	ID          string `json:"id"`
	Agent       string `json:"agent"`
	Repo        string `json:"repo"`
	Name        string `json:"name,omitempty"`
	When        string `json:"when"`
	LastActive  string `json:"last_active"`
	Recap       string `json:"recap,omitempty"`
	FirstPrompt string `json:"first_prompt,omitempty"`
	LastPrompt  string `json:"last_prompt,omitempty"`
}

// searchMatch is one item in the LLM response.
type searchMatch struct {
	ID     string `json:"id"`
	Reason string `json:"reason"`
}

type searchResponse struct {
	Matches []searchMatch `json:"matches"`
}

// AgentSearch finds agent sessions matching a natural-language query by
// shelling out to a configurable LLM CLI. The CLI receives a JSON catalog
// of recent sessions on stdin and returns a ranked list of session IDs.
func AgentSearch(m *manifest.Manifest, wsHome, query string, opts AgentSearchOptions) error {
	query = strings.TrimSpace(query)
	if query == "" {
		return fmt.Errorf("search query cannot be empty")
	}

	pathIndex := buildPathIndex(m, wsHome, "", false)
	sessions := discoverAllSessions(pathIndex, opts.External)
	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No agent sessions found in this workspace.")
		return nil
	}

	catalogMax := opts.CatalogMax
	if catalogMax <= 0 {
		catalogMax = agentSearchDefaultCatalog
	}
	if len(sessions) > catalogMax {
		sessions = sessions[:catalogMax]
	}

	enrichSessionVerboseInfo(sessions)

	maxResults := opts.MaxResults
	if maxResults <= 0 {
		maxResults = agentSearchDefaultResults
	}

	prompt := buildSearchPrompt(query, sessions, opts.Verbose, maxResults)
	cmdLine := resolveSearcherCmd(m)

	fmt.Fprintf(os.Stderr, "Searching %d sessions via `%s`...\n", len(sessions), cmdLine)

	output, err := runSearcher(cmdLine, prompt)
	if err != nil {
		return fmt.Errorf("running searcher: %w", err)
	}

	matches, err := parseSearchResponse(output)
	if err != nil {
		// Surface the raw output so the user can see what went wrong.
		fmt.Fprintln(os.Stderr, "Searcher output:")
		fmt.Fprintln(os.Stderr, strings.TrimSpace(output))
		return fmt.Errorf("parsing searcher response: %w", err)
	}

	printSearchResults(sessions, matches)
	return nil
}

// resolveSearcherCmd picks the LLM command line: env > manifest > default.
func resolveSearcherCmd(m *manifest.Manifest) string {
	if env := strings.TrimSpace(os.Getenv(agentSearchEnvCmd)); env != "" {
		return env
	}
	if m != nil && m.Agents != nil {
		if cmd, ok := m.Agents[agentSearchManifestKey]; ok && strings.TrimSpace(cmd) != "" {
			return cmd
		}
	}
	return agentSearchDefaultCmd
}

// buildSearchPrompt assembles the prompt sent to the LLM. Catalog is embedded
// as JSON so the model has structured fields to reason about.
func buildSearchPrompt(query string, sessions []AgentSession, verbose bool, maxResults int) string {
	catalog := buildSearchCatalog(sessions, verbose)
	catalogJSON, _ := json.Marshal(catalog)

	verboseLine := ""
	if verbose {
		verboseLine = "\n- last_prompt: most recent user message (may be empty)"
	}

	return fmt.Sprintf(`TASK: Find agent sessions matching the user's query.

USER QUERY: %s

Each catalog entry has:
- id: full session ID
- agent: claude or codex
- repo: repo name
- name: user-set session name (may be empty)
- when: relative time (e.g. "3h ago")
- last_active: ISO timestamp
- recap: agent-written summary (may be empty)
- first_prompt: first user message (may be empty)%s

CATALOG (JSON, sorted most-recent first):
%s

INSTRUCTIONS:
- Output ONLY JSON. No markdown fence, no preamble, no commentary.
- Format: {"matches":[{"id":"<full id>","reason":"<one short sentence>"}]}
- Rank most-relevant first, max %d matches.
- Skip sessions you're unsure about — quality over quantity.
- If nothing matches, output {"matches":[]}.
`, query, verboseLine, string(catalogJSON), maxResults)
}

// buildSearchCatalog projects sessions to compact catalog entries for the LLM.
func buildSearchCatalog(sessions []AgentSession, verbose bool) []catalogEntry {
	out := make([]catalogEntry, 0, len(sessions))
	for _, s := range sessions {
		entry := catalogEntry{
			ID:          s.SessionID,
			Agent:       s.Agent,
			Repo:        s.Repo,
			Name:        s.Name,
			When:        formatTimeAgo(time.Now(), s.LastActive),
			LastActive:  s.LastActive.UTC().Format(time.RFC3339),
			Recap:       cleanPromptText(s.Summary),
			FirstPrompt: cleanPromptText(s.Prompt),
		}
		if verbose {
			entry.LastPrompt = cleanPromptText(s.LastPrompt)
		}
		out = append(out, entry)
	}
	return out
}

// runSearcher executes the configured LLM CLI under `sh -c` so the user can
// supply a pipeline. The full prompt is fed via stdin to avoid argv size
// limits and to keep quoting simple.
func runSearcher(cmdLine, prompt string) (string, error) {
	cmd := exec.Command("sh", "-c", cmdLine)
	cmd.Stdin = strings.NewReader(prompt)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		errOut := strings.TrimSpace(stderr.String())
		if errOut != "" {
			return stdout.String(), fmt.Errorf("%w: %s", err, errOut)
		}
		return stdout.String(), err
	}
	return stdout.String(), nil
}

// parseSearchResponse extracts a searchResponse from raw LLM output. Tolerates
// surrounding markdown fences or commentary by isolating the first balanced
// JSON object.
func parseSearchResponse(raw string) ([]searchMatch, error) {
	jsonBlob, err := extractJSONObject(raw)
	if err != nil {
		return nil, err
	}
	var resp searchResponse
	if err := json.Unmarshal([]byte(jsonBlob), &resp); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return resp.Matches, nil
}

// extractJSONObject returns the substring of s containing the first balanced
// top-level JSON object. Skips characters inside string literals.
func extractJSONObject(s string) (string, error) {
	start := strings.IndexByte(s, '{')
	if start < 0 {
		return "", fmt.Errorf("no JSON object found in output")
	}

	depth := 0
	inString := false
	escaped := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escaped {
				escaped = false
				continue
			}
			switch c {
			case '\\':
				escaped = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unterminated JSON object in output")
}

// printSearchResults prints matched sessions in a table similar to `agent ls`,
// with an extra REASON column pulled from the LLM response.
func printSearchResults(sessions []AgentSession, matches []searchMatch) {
	if len(matches) == 0 {
		fmt.Fprintln(os.Stderr, "No matches.")
		return
	}

	bySession := make(map[string]AgentSession, len(sessions))
	for _, s := range sessions {
		bySession[s.SessionID] = s
	}

	rows := make([]matchedRow, 0, len(matches))
	for _, m := range matches {
		s, ok := lookupSessionForMatch(bySession, m.ID)
		if !ok {
			continue
		}
		rows = append(rows, matchedRow{Session: s, Reason: m.Reason})
	}

	if len(rows) == 0 {
		fmt.Fprintln(os.Stderr, "Searcher returned matches but none mapped to known sessions.")
		return
	}

	renderSearchTable(rows)
	fmt.Printf("\n%s\n", term.Colorize(term.Dim, "Resume: ws agent resume <id-prefix>"))
}

type matchedRow struct {
	Session AgentSession
	Reason  string
}

// lookupSessionForMatch matches by full ID first, then falls back to a unique
// prefix match in case the LLM truncated the ID.
func lookupSessionForMatch(by map[string]AgentSession, id string) (AgentSession, bool) {
	if s, ok := by[id]; ok {
		return s, true
	}
	if id == "" {
		return AgentSession{}, false
	}
	var found AgentSession
	matches := 0
	for sid, s := range by {
		if strings.HasPrefix(sid, id) {
			found = s
			matches++
		}
	}
	if matches == 1 {
		return found, true
	}
	return AgentSession{}, false
}

// renderSearchTable lays out matched sessions as #/AGENT/REPO/WHEN/ID/REASON.
// The REASON column gets whatever terminal width remains.
func renderSearchTable(rows []matchedRow) {
	now := time.Now()
	termWidth, _, _ := xterm.GetSize(int(os.Stdout.Fd()))

	maxRepo := len("REPO")
	for _, r := range rows {
		if n := len(r.Session.Repo); n > maxRepo {
			maxRepo = n
		}
	}
	if maxRepo > 24 {
		maxRepo = 24
	}

	idxWidth := len(fmt.Sprintf("%d", len(rows)))
	if idxWidth < 2 {
		idxWidth = 2
	}

	const idCol = 8

	fmt.Printf("%*s  %-8s %-*s  %-12s %-*s  %s\n",
		idxWidth, "#", "AGENT", maxRepo, "REPO", "WHEN", idCol, "ID", "REASON")
	fmt.Println(strings.Repeat("-", 60+maxRepo))

	for i, r := range rows {
		when := formatTimeAgo(now, r.Session.LastActive)
		shortID := shortSessionID(r.Session.SessionID)
		if len(shortID) > idCol {
			shortID = shortID[:idCol]
		}

		reasonWidth := 40
		if termWidth > 0 {
			prefix := idxWidth + 2 + 8 + 1 + maxRepo + 2 + 12 + 1 + idCol + 2
			reasonWidth = termWidth - prefix
			if reasonWidth < 20 {
				reasonWidth = 20
			}
		}

		fmt.Printf("%*d  %s %-*s  %-12s %-*s  %s\n",
			idxWidth, i+1,
			term.Colorize(agentTypeColor(r.Session.Agent), fmt.Sprintf("%-8s", r.Session.Agent)),
			maxRepo, truncateText(r.Session.Repo, maxRepo),
			term.Colorize(term.Dim, when),
			idCol, shortID,
			truncateText(r.Reason, reasonWidth),
		)
	}
}

