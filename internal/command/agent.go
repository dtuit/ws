package command

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
	xterm "golang.org/x/term"
)

// AgentSession describes a discovered AI agent session.
type AgentSession struct {
	Agent            string    // "claude", "codex"
	SessionID        string    // agent-specific session identifier
	Repo             string    // repo name; workspace-root sessions use the workspace basename
	Dir              string    // absolute path where the session was started
	StartedAt        time.Time // first activity
	LastActive       time.Time // most recent activity
	Prompt            string   // first user message
	Prompts           []string // first few user messages (for verbose display)
	Summary           string   // away_summary / recap (if available)
	LastPrompt        string   // most recent user message (if available)
	Model             string   // model used (optional)
	Name              string   // user-set session name (e.g., Claude /rename)
	Active            bool     // true if the agent process is still running
	BypassPermissions bool     // session was launched with permission bypass (e.g., --dangerously-skip-permissions)
	Pinned            bool     // session is pinned in the workspace pins file
}

const (
	AgentDefaultLimit = 20
	agentClaude       = "claude"
	agentCodex        = "codex"

	// Special filter tokens for `ws agent ls`
	agentFilterCwd      = "."        // current directory
	agentFilterRoot     = "root"     // workspace root sessions only
	agentFilterExternal = "external" // sessions outside the workspace
)

// workspaceRootRepoName returns the label used in the REPO column for
// sessions started at the workspace root. Defaults to the workspace
// directory's basename so listings stay informative; falls back to
// "workspace" if wsHome is empty / "/" / ".".
func workspaceRootRepoName(wsHome string) string {
	base := filepath.Base(wsHome)
	if base == "" || base == "." || base == "/" {
		return "workspace"
	}
	return base
}

// AgentListMode controls optional agent list output extensions.
type AgentListMode struct {
	Verbose   bool
	ShowLast  bool // compact view: show last user prompt instead of first
	ShowRecap bool // compact view: show recap, falling back to last/first
}

// needsEnrichment reports whether the mode requires reading conversation
// details (Summary/LastPrompt) beyond what discovery already collects.
func (m AgentListMode) needsEnrichment() bool {
	return m.Verbose || m.ShowLast || m.ShowRecap
}

// AgentList discovers and prints agent sessions across the workspace.
func AgentList(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool, limit int, mode AgentListMode) error {
	filter, err := resolveAgentListFilter(m, wsHome, filter)
	if err != nil {
		return err
	}

	// External mode: discover sessions outside the workspace.
	// Build the full workspace path index so we can exclude matches.
	external := filter == agentFilterExternal
	pathIndexFilter := filter
	if external {
		pathIndexFilter = ""
	}
	pathIndex := buildPathIndex(m, wsHome, pathIndexFilter, includeWorktrees)

	sessions := discoverAllSessions(pathIndex, external)

	// Post-filter for special tokens that buildPathIndex can't express
	if filter == agentFilterRoot {
		sessions = filterSessionsByRepo(sessions, workspaceRootRepoName(wsHome))
	}

	if len(sessions) == 0 {
		fmt.Fprintln(os.Stderr, "No agent sessions found in this workspace.")
		return nil
	}

	pins, _ := loadAgentPins(wsHome)
	sessions = applyAgentPins(sessions, pins)

	if limit > 0 && len(sessions) > limit {
		sessions = truncatePreservingPins(sessions, limit)
	}

	if mode.needsEnrichment() {
		enrichSessionVerboseInfo(sessions)
	}

	printAgentSessions(sessions, mode)
	return nil
}

// AgentResume outputs the directory and resume command for a session,
// identified by numeric index (1-based) or partial session ID.
func AgentResume(m *manifest.Manifest, wsHome string, indexOrID string) error {
	pathIndex := buildPathIndex(m, wsHome, "", false)
	sessions := discoverAllSessions(pathIndex, false)

	if len(sessions) == 0 {
		return fmt.Errorf("no agent sessions found in this workspace")
	}

	session, err := resolveSessionRef(sessions, indexOrID)
	if err != nil {
		return err
	}

	resumeCmd := agentResumeCmd(m, session)
	fmt.Println(session.Dir)
	fmt.Println(resumeCmd)
	return nil
}

// AgentPin adds a session to the workspace pins file. If ref is empty,
// the current session is detected by walking ancestor process PIDs.
func AgentPin(m *manifest.Manifest, wsHome, ref string) error {
	sessionID, label, err := resolvePinTarget(m, wsHome, ref)
	if err != nil {
		return err
	}
	added, err := addAgentPin(wsHome, sessionID)
	if err != nil {
		return err
	}
	if !added {
		fmt.Fprintf(os.Stderr, "Already pinned: %s\n", label)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Pinned %s\n", label)
	return nil
}

// AgentUnpin removes a session from the workspace pins file. If ref is
// empty, the current session is detected by walking ancestor process PIDs.
func AgentUnpin(m *manifest.Manifest, wsHome, ref string) error {
	sessionID, label, err := resolvePinTarget(m, wsHome, ref)
	if err != nil {
		return err
	}
	removed, err := removeAgentPin(wsHome, sessionID)
	if err != nil {
		return err
	}
	if !removed {
		fmt.Fprintf(os.Stderr, "Not pinned: %s\n", label)
		return nil
	}
	fmt.Fprintf(os.Stderr, "Unpinned %s\n", label)
	return nil
}

// resolvePinTarget returns (sessionID, displayLabel, err). If ref is empty,
// uses current-session detection. Otherwise resolves the ref against the
// list of discovered sessions (numeric index or ID prefix). When detection
// is used and no session record matches, returns the raw detected ID.
func resolvePinTarget(m *manifest.Manifest, wsHome, ref string) (string, string, error) {
	if ref == "" {
		sid, ok := detectCurrentAgentSession()
		if !ok {
			return "", "", fmt.Errorf("not inside an agent session; pass <#|id> explicitly")
		}
		// Try to enrich with repo/agent info for the confirmation message.
		pathIndex := buildPathIndex(m, wsHome, "", false)
		sessions := discoverAllSessions(pathIndex, false)
		for _, s := range sessions {
			if s.SessionID == sid {
				return sid, pinLabel(s), nil
			}
		}
		return sid, shortSessionID(sid), nil
	}

	pathIndex := buildPathIndex(m, wsHome, "", false)
	sessions := discoverAllSessions(pathIndex, false)
	if len(sessions) == 0 {
		return "", "", fmt.Errorf("no agent sessions found in this workspace")
	}
	pins, _ := loadAgentPins(wsHome)
	sessions = applyAgentPins(sessions, pins)

	session, err := resolveSessionRef(sessions, ref)
	if err != nil {
		return "", "", err
	}
	return session.SessionID, pinLabel(session), nil
}

func pinLabel(s AgentSession) string {
	return fmt.Sprintf("%s (%s/%s)", shortSessionID(s.SessionID), s.Agent, s.Repo)
}

func shortSessionID(id string) string {
	if len(id) > 12 {
		return id[:12]
	}
	return id
}

// AgentStart outputs the directory and start command for an agent.
// If repo is empty, uses the current directory.
func AgentStart(m *manifest.Manifest, wsHome, repo, agentName string, passthrough []string) error {
	dir, err := resolveAgentDir(m, wsHome, repo)
	if err != nil {
		return err
	}

	cmd := resolveAgentCmd(m, agentName)
	if len(passthrough) > 0 {
		cmd = cmd + " " + shellJoin(passthrough)
	}

	fmt.Println(dir)
	fmt.Println(cmd)
	return nil
}

// resolveAgentDir returns the absolute directory for a repo name,
// or the current directory if repo is empty.
func resolveAgentDir(m *manifest.Manifest, wsHome, repo string) (string, error) {
	if repo == "" {
		return os.Getwd()
	}

	active := m.ActiveRepos()
	cfg, ok := active[repo]
	if !ok {
		return "", fmt.Errorf("unknown repo: %s", repo)
	}
	return m.ResolvePath(wsHome, repo, cfg), nil
}

// resolveAgentCmd resolves the agent name to a shell command string.
// Priority: manifest agents config → WS_AGENT env → bare name.
func resolveAgentCmd(m *manifest.Manifest, name string) string {
	if name == "" {
		name = resolveDefaultAgentName(m)
	}

	// Check manifest agents config
	if m != nil && m.Agents != nil {
		if cmd, ok := m.Agents[name]; ok {
			return cmd
		}
	}

	return name
}

// resolveDefaultAgentName returns the default agent profile name.
func resolveDefaultAgentName(m *manifest.Manifest) string {
	// WS_AGENT env var takes precedence
	if env := os.Getenv("WS_AGENT"); env != "" {
		return env
	}

	// Then check manifest agents.default
	if m != nil && m.Agents != nil {
		if def, ok := m.Agents["default"]; ok {
			return def
		}
	}

	return agentClaude
}

// agentResumeCmd builds the resume command for a session, using the
// configured agent command as the base but ensuring permission flags
// match the original session.
func agentResumeCmd(m *manifest.Manifest, s AgentSession) string {
	base := resolveAgentCmd(m, s.Agent)

	switch s.Agent {
	case agentClaude:
		base = reconcileClaudePermissionFlag(base, s.BypassPermissions)
		return base + " --resume " + s.SessionID
	case agentCodex:
		return base + " resume " + s.SessionID
	default:
		return base + " --resume " + s.SessionID
	}
}

// reconcileClaudePermissionFlag ensures --dangerously-skip-permissions is
// present or absent in the command string to match the original session.
func reconcileClaudePermissionFlag(cmd string, needsBypass bool) string {
	const flag = "--dangerously-skip-permissions"
	hasFlag := strings.Contains(cmd, flag)

	if needsBypass && !hasFlag {
		return cmd + " " + flag
	}
	if !needsBypass && hasFlag {
		// Remove the flag, cleaning up extra spaces
		cmd = strings.ReplaceAll(cmd, " "+flag, "")
		cmd = strings.ReplaceAll(cmd, flag+" ", "")
		cmd = strings.ReplaceAll(cmd, flag, "")
		return strings.TrimSpace(cmd)
	}
	return cmd
}

// buildPathIndex creates a mapping from absolute repo paths to repo names,
// used to match agent session directories to workspace repos.
func buildPathIndex(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) map[string]string {
	index := make(map[string]string)

	if filter == agentFilterRoot {
		index[wsHome] = workspaceRootRepoName(wsHome)
		return index
	}

	var repos []manifest.RepoInfo
	if filter == "" {
		index[wsHome] = workspaceRootRepoName(wsHome)
		repos = m.AllRepos(wsHome)
	} else {
		var err error
		repos, err = resolveCommandRepos(m, wsHome, filter, includeWorktrees)
		if err != nil {
			repos = m.AllRepos(wsHome)
		}
	}

	for _, repo := range repos {
		index[repo.Path] = repo.Name
	}
	return index
}

// matchSessionRepo finds the repo name for a session directory by
// matching against the path index (longest prefix match).
func matchSessionRepo(dir string, pathIndex map[string]string) (string, bool) {
	dir = filepath.Clean(dir)

	// Exact match first
	if name, ok := pathIndex[dir]; ok {
		return name, true
	}

	// Longest prefix match (for subdirectories)
	bestMatch := ""
	bestLen := 0
	for path, name := range pathIndex {
		if strings.HasPrefix(dir, path+string(filepath.Separator)) && len(path) > bestLen {
			bestMatch = name
			bestLen = len(path)
		}
	}

	if bestMatch != "" {
		return bestMatch, true
	}
	return "", false
}

// resolveAgentListFilter converts special filter tokens to concrete values.
// "." becomes the repo name (or "root") matching the current directory.
// "root" is passed through and handled post-discovery.
func resolveAgentListFilter(m *manifest.Manifest, wsHome, filter string) (string, error) {
	if filter != agentFilterCwd {
		return filter, nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("resolving current directory: %w", err)
	}

	// Build a full path index (no filter) to find what contains cwd
	fullIndex := buildPathIndex(m, wsHome, "", false)
	name, ok := matchSessionRepo(cwd, fullIndex)
	if !ok {
		return "", fmt.Errorf("current directory is not inside the workspace (%s)", wsHome)
	}

	if name == workspaceRootRepoName(wsHome) {
		return agentFilterRoot, nil
	}
	return name, nil
}

// externalRepoLabel derives a display label for a session directory that is
// outside the current workspace. Uses the basename, with ~ substitution
// for the user's home directory to keep things readable.
func externalRepoLabel(dir string) string {
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		if dir == home {
			return "~"
		}
		if strings.HasPrefix(dir, home+string(filepath.Separator)) {
			rel := strings.TrimPrefix(dir, home+string(filepath.Separator))
			// Collapse deep paths: show the last 2 components
			parts := strings.Split(rel, string(filepath.Separator))
			if len(parts) > 2 {
				return ".../" + filepath.Join(parts[len(parts)-2:]...)
			}
			return "~/" + rel
		}
	}
	base := filepath.Base(dir)
	if base == "" || base == "/" {
		return dir
	}
	return base
}

// applyAgentPins marks sessions with the Pinned flag and sorts pinned
// sessions first, preserving the caller's original order within each group.
func applyAgentPins(sessions []AgentSession, pins map[string]bool) []AgentSession {
	for i := range sessions {
		if pins[sessions[i].SessionID] {
			sessions[i].Pinned = true
		}
	}
	sort.SliceStable(sessions, func(i, j int) bool {
		return sessions[i].Pinned && !sessions[j].Pinned
	})
	return sessions
}

// truncatePreservingPins returns the first `limit` sessions, ensuring all
// pinned sessions are included even if they would otherwise fall past it.
func truncatePreservingPins(sessions []AgentSession, limit int) []AgentSession {
	if limit <= 0 || len(sessions) <= limit {
		return sessions
	}

	pinned := make([]AgentSession, 0)
	unpinned := make([]AgentSession, 0, len(sessions))
	for _, s := range sessions {
		if s.Pinned {
			pinned = append(pinned, s)
		} else {
			unpinned = append(unpinned, s)
		}
	}

	remaining := limit - len(pinned)
	if remaining < 0 {
		remaining = 0
	}
	if remaining < len(unpinned) {
		unpinned = unpinned[:remaining]
	}
	return append(pinned, unpinned...)
}

func filterSessionsByRepo(sessions []AgentSession, repoName string) []AgentSession {
	filtered := make([]AgentSession, 0, len(sessions))
	for _, s := range sessions {
		if s.Repo == repoName {
			filtered = append(filtered, s)
		}
	}
	return filtered
}

func enrichSessionVerboseInfo(sessions []AgentSession) {
	home, err := os.UserHomeDir()
	if err != nil {
		return
	}
	claudeDir := filepath.Join(home, ".claude")

	for i := range sessions {
		switch sessions[i].Agent {
		case agentClaude:
			enrichClaudeSessionDetail(&sessions[i], claudeDir)
		}
	}
}

func discoverAllSessions(pathIndex map[string]string, external bool) []AgentSession {
	var sessions []AgentSession

	if cs := discoverClaudeSessions(pathIndex, external); len(cs) > 0 {
		sessions = append(sessions, cs...)
	}
	if cs := discoverCodexSessions(pathIndex, external); len(cs) > 0 {
		sessions = append(sessions, cs...)
	}

	// Sort by LastActive descending
	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].LastActive.After(sessions[j].LastActive)
	})

	return sessions
}

func resolveSessionRef(sessions []AgentSession, ref string) (AgentSession, error) {
	// Try as numeric index (1-based)
	if idx, err := strconv.Atoi(ref); err == nil {
		if idx < 1 || idx > len(sessions) {
			return AgentSession{}, fmt.Errorf("index %d out of range (1-%d)", idx, len(sessions))
		}
		return sessions[idx-1], nil
	}

	// Try as session ID prefix
	var matches []AgentSession
	for _, s := range sessions {
		if strings.HasPrefix(s.SessionID, ref) {
			matches = append(matches, s)
		}
	}

	switch len(matches) {
	case 0:
		return AgentSession{}, fmt.Errorf("no session matching %q", ref)
	case 1:
		return matches[0], nil
	default:
		var ids []string
		for _, s := range matches {
			id := s.SessionID
			if len(id) > 12 {
				id = id[:12] + "..."
			}
			ids = append(ids, id)
		}
		return AgentSession{}, fmt.Errorf("ambiguous session %q, matches: %s", ref, strings.Join(ids, ", "))
	}
}

func printAgentSessions(sessions []AgentSession, mode AgentListMode) {
	now := time.Now()
	termWidth, _, _ := xterm.GetSize(int(os.Stdout.Fd()))

	// Compute column widths
	maxRepo := len("REPO")
	for _, s := range sessions {
		if n := utf8.RuneCountInString(s.Repo); n > maxRepo {
			maxRepo = n
		}
	}
	if maxRepo > 30 {
		maxRepo = 30
	}

	idxWidth := len(strconv.Itoa(len(sessions)))
	if idxWidth < 2 {
		idxWidth = 2
	}

	if mode.Verbose {
		printAgentSessionsVerbose(sessions, now, termWidth, maxRepo, idxWidth)
	} else {
		printAgentSessionsCompact(sessions, now, termWidth, maxRepo, idxWidth, mode)
	}

	fmt.Printf("\n%s\n", term.Colorize(term.Dim, "Resume: ws agent resume <#>"))
}

func printAgentSessionsCompact(sessions []AgentSession, now time.Time, termWidth, maxRepo, idxWidth int, mode AgentListMode) {
	header := compactHeaderLabel(mode)
	fmt.Printf("%*s  %-8s %-*s  %-12s %s\n",
		idxWidth, "#", "AGENT", maxRepo, "REPO", "WHEN", header)
	fmt.Println(strings.Repeat("-", 60+maxRepo))

	for i, s := range sessions {
		activeMarker := " "
		switch {
		case s.Pinned && s.Active:
			activeMarker = term.Colorize(term.Yellow, "P")
		case s.Pinned:
			activeMarker = term.Colorize(term.Yellow, "p")
		case s.Active:
			activeMarker = "*"
		}

		when := formatTimeAgo(now, s.LastActive)

		promptWidth := 40
		if termWidth > 0 {
			prefix := idxWidth + 2 + 8 + 1 + maxRepo + 2 + 1 + 12 + 1
			promptWidth = termWidth - prefix
			if promptWidth < 20 {
				promptWidth = 20
			}
		}

		var namePrefix string
		textWidth := promptWidth
		if s.Name != "" {
			nameTag := "[" + s.Name + "]"
			// Keep at least some room for the prompt text even when the
			// name is long.
			nameBudget := promptWidth - 6
			if nameBudget < len(nameTag) {
				nameTag = truncateText(nameTag, nameBudget)
			}
			namePrefix = term.Colorize(term.Cyan, nameTag) + " "
			textWidth = promptWidth - utf8.RuneCountInString(nameTag) - 1
			if textWidth < 0 {
				textWidth = 0
			}
		}
		prompt := namePrefix + truncateText(selectCompactText(s, mode), textWidth)

		fmt.Printf("%*d  %s %-*s %s%-12s %s\n",
			idxWidth, i+1,
			term.Colorize(agentTypeColor(s.Agent), fmt.Sprintf("%-8s", s.Agent)),
			maxRepo, truncateText(s.Repo, maxRepo),
			activeMarker,
			term.Colorize(term.Dim, when),
			prompt,
		)
	}
}

// selectCompactText picks the text shown in the compact PROMPT column
// based on the list mode, falling back through recap → last → first when
// the preferred source is empty.
func selectCompactText(s AgentSession, mode AgentListMode) string {
	if mode.ShowRecap {
		if t := cleanPromptText(s.Summary); t != "" {
			return t
		}
		if t := cleanPromptText(s.LastPrompt); t != "" {
			return t
		}
		return s.Prompt
	}
	if mode.ShowLast {
		if t := cleanPromptText(s.LastPrompt); t != "" {
			return t
		}
		return s.Prompt
	}
	return s.Prompt
}

func compactHeaderLabel(mode AgentListMode) string {
	switch {
	case mode.ShowRecap:
		return "RECAP"
	case mode.ShowLast:
		return "LAST"
	default:
		return "PROMPT"
	}
}

func printAgentSessionsVerbose(sessions []AgentSession, now time.Time, termWidth, maxRepo, idxWidth int) {
	wrapWidth := 80
	if termWidth > 0 {
		wrapWidth = termWidth - idxWidth - 4
		if wrapWidth < 40 {
			wrapWidth = 40
		}
	}

	indent := strings.Repeat(" ", idxWidth+2)

	for i, s := range sessions {
		markers := ""
		if s.Pinned {
			markers += " " + term.Colorize(term.Yellow, "[pinned]")
		}
		if s.Active {
			markers += " *"
		}

		when := formatTimeAgo(now, s.LastActive)

		nameLabel := ""
		if s.Name != "" {
			nameLabel = "  " + term.Colorize(term.Cyan, "["+s.Name+"]")
		}

		// Header line: index, agent, repo, name, when
		fmt.Printf("%*d  %s  %s%s  %s%s\n",
			idxWidth, i+1,
			term.Colorize(agentTypeColor(s.Agent), s.Agent),
			s.Repo,
			nameLabel,
			term.Colorize(term.Dim, when),
			markers,
		)

		lastText := cleanPromptText(s.LastPrompt)
		lastShownInline := false

		// Summary (recap) if available — most useful context
		if s.Summary != "" {
			fmt.Printf("%s%s\n", indent, term.Colorize(term.Dim, "Recap:"))
			for _, line := range wrapText(cleanPromptText(s.Summary), wrapWidth) {
				fmt.Printf("%s%s\n", indent, line)
			}
		} else {
			// Fall back to first few user prompts
			prompts := s.Prompts
			if len(prompts) == 0 && s.Prompt != "" {
				prompts = []string{s.Prompt}
			}
			for j, p := range prompts {
				promptText := cleanPromptText(p)
				if promptText == "" {
					continue
				}
				for _, line := range wrapText(promptText, wrapWidth) {
					fmt.Printf("%s%s\n", indent, line)
				}
				if lastText != "" && promptText == lastText {
					fmt.Printf("%s%s\n", indent, term.Colorize(term.Dim, "(last)"))
					lastShownInline = true
				}
				if j < len(prompts)-1 {
					fmt.Printf("%s%s\n", indent, term.Colorize(term.Dim, "---"))
				}
			}
		}

		// Fall back to a separate block only when LastPrompt didn't match
		// any prompt we already rendered inline (and isn't the summary).
		if lastText != "" && !lastShownInline && cleanPromptText(s.Summary) != lastText {
			fmt.Printf("%s%s\n", indent, term.Colorize(term.Dim, "Last:"))
			for _, line := range wrapText(lastText, wrapWidth) {
				fmt.Printf("%s%s\n", indent, line)
			}
		}

		// Blank line between entries
		if i < len(sessions)-1 {
			fmt.Println()
		}
	}
}

// cleanPromptText normalizes whitespace in a prompt for display.
func cleanPromptText(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "\t", "  ")
	return strings.TrimSpace(s)
}

// wrapText wraps a string to the given width, preserving existing newlines.
func wrapText(s string, width int) []string {
	if width <= 0 {
		return []string{s}
	}

	var result []string
	for _, paragraph := range strings.Split(s, "\n") {
		paragraph = strings.TrimRight(paragraph, " ")
		if paragraph == "" {
			result = append(result, "")
			continue
		}
		for len(paragraph) > 0 {
			if utf8.RuneCountInString(paragraph) <= width {
				result = append(result, paragraph)
				break
			}
			// Find a break point (space) near the width
			runes := []rune(paragraph)
			breakAt := width
			for breakAt > 0 && runes[breakAt] != ' ' {
				breakAt--
			}
			if breakAt == 0 {
				// No space found, hard break
				breakAt = width
			}
			result = append(result, string(runes[:breakAt]))
			paragraph = strings.TrimLeft(string(runes[breakAt:]), " ")
		}
	}
	return result
}

func agentTypeColor(agent string) string {
	switch agent {
	case agentClaude:
		return term.Cyan
	case agentCodex:
		return term.Green
	default:
		return term.Yellow
	}
}

func formatTimeAgo(now time.Time, t time.Time) string {
	if t.IsZero() {
		return ""
	}

	d := now.Sub(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	case d < 7*24*time.Hour:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	case d < 30*24*time.Hour:
		return fmt.Sprintf("%dw ago", int(d.Hours()/(24*7)))
	default:
		return fmt.Sprintf("%dmo ago", int(d.Hours()/(24*30)))
	}
}

func truncateText(s string, maxWidth int) string {
	if maxWidth <= 0 {
		return ""
	}
	// Replace newlines/tabs with spaces
	s = strings.NewReplacer("\n", " ", "\r", "", "\t", " ").Replace(s)
	s = strings.TrimSpace(s)

	if utf8.RuneCountInString(s) <= maxWidth {
		return s
	}
	if maxWidth <= 3 {
		return s[:maxWidth]
	}
	runes := []rune(s)
	return string(runes[:maxWidth-3]) + "..."
}

func shellJoin(args []string) string {
	parts := make([]string, len(args))
	for i, arg := range args {
		if strings.ContainsAny(arg, " \t\n\"'\\$`!#&|;(){}[]<>?*~") {
			parts[i] = "'" + strings.ReplaceAll(arg, "'", "'\\''") + "'"
		} else {
			parts[i] = arg
		}
	}
	return strings.Join(parts, " ")
}
