package command

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type claudeHistoryEntry struct {
	Display   string `json:"display"`
	Timestamp int64  `json:"timestamp"` // milliseconds
	Project   string `json:"project"`
	SessionID string `json:"sessionId"`
}

type claudeSessionMeta struct {
	PID       int    `json:"pid"`
	SessionID string `json:"sessionId"`
	Name      string `json:"name"`
	UpdatedAt int64  `json:"updatedAt"` // milliseconds
}

// discoverClaudeSessions reads ~/.claude/history.jsonl and returns sessions
// whose project paths match the workspace path index. When external is true,
// returns sessions whose project paths do NOT match (for discovering sessions
// from outside the current workspace).
func discoverClaudeSessions(pathIndex map[string]string, external bool) []AgentSession {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	historyPath := filepath.Join(home, ".claude", "history.jsonl")
	f, err := os.Open(historyPath)
	if err != nil {
		return nil
	}
	defer f.Close()

	// Group history entries by session ID.
	// history.jsonl is append-only and chronological, so first occurrence
	// of a sessionId is the earliest message and last is the most recent.
	const maxPrompts = 3 // collect first N user messages for verbose context

	type sessionAccum struct {
		project  string
		prompt   string   // first user message
		prompts  []string // first N user messages (for verbose mode)
		firstTS  int64
		lastTS   int64
	}

	accum := make(map[string]*sessionAccum)
	// Track insertion order so output is deterministic
	var order []string

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 256*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var entry claudeHistoryEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.SessionID == "" || entry.Project == "" {
			continue
		}

		if s, ok := accum[entry.SessionID]; ok {
			if entry.Timestamp > s.lastTS {
				s.lastTS = entry.Timestamp
			}
			if len(s.prompts) < maxPrompts && entry.Display != "" {
				s.prompts = append(s.prompts, entry.Display)
			}
		} else {
			prompts := []string{}
			if entry.Display != "" {
				prompts = append(prompts, entry.Display)
			}
			accum[entry.SessionID] = &sessionAccum{
				project: entry.Project,
				prompt:  entry.Display,
				prompts: prompts,
				firstTS: entry.Timestamp,
				lastTS:  entry.Timestamp,
			}
			order = append(order, entry.SessionID)
		}
	}

	// Filter sessions by workspace paths and verify conversation file exists
	claudeDir := filepath.Join(home, ".claude")
	var sessions []AgentSession
	for _, sid := range order {
		s := accum[sid]
		repo, matched := matchSessionRepo(s.project, pathIndex)
		if external {
			if matched {
				continue // in-workspace sessions are excluded in external mode
			}
			repo = externalRepoLabel(s.project)
		} else if !matched {
			continue
		}

		// Only include sessions that have a conversation file on disk.
		// Short-lived sessions (e.g., /resume meta-commands) may appear
		// in history.jsonl but have no persisted conversation to resume.
		sessionMeta, ok := readClaudeSessionMeta(claudeDir, s.project, sid)
		if !ok {
			continue
		}

		sessions = append(sessions, AgentSession{
			Agent:             agentClaude,
			SessionID:         sid,
			Repo:              repo,
			Dir:               s.project,
			StartedAt:         time.UnixMilli(s.firstTS),
			LastActive:        time.UnixMilli(s.lastTS),
			Prompt:            s.prompt,
			Prompts:           s.prompts,
			BypassPermissions: sessionMeta.bypassPermissions,
		})
	}

	// Enrich with active-process information and names from session metadata
	enrichClaudeSessionMetadata(sessions, home)

	return sessions
}

// enrichClaudeSessionMetadata scans ~/.claude/sessions/*.json to mark
// sessions that are currently running (PID is alive) and to populate
// user-set session names (from the /rename command). The name lives in
// the metadata file; when multiple files exist for one session ID, the
// most recently updated one wins.
func enrichClaudeSessionMetadata(sessions []AgentSession, home string) {
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return
	}

	liveSessionIDs := make(map[string]bool)

	type nameEntry struct {
		name      string
		updatedAt int64
	}
	namesBySessionID := make(map[string]nameEntry)

	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(sessionsDir, entry.Name()))
		if err != nil {
			continue
		}

		var meta claudeSessionMeta
		if err := json.Unmarshal(data, &meta); err != nil || meta.SessionID == "" {
			continue
		}

		if meta.PID > 0 && isProcessAlive(meta.PID) {
			liveSessionIDs[meta.SessionID] = true
		}

		// Keep the most recently updated record per session ID so that a
		// subsequent /rename (or clear) wins over older files.
		prev, seen := namesBySessionID[meta.SessionID]
		if !seen || meta.UpdatedAt > prev.updatedAt {
			namesBySessionID[meta.SessionID] = nameEntry{name: meta.Name, updatedAt: meta.UpdatedAt}
		}
	}

	for i := range sessions {
		if liveSessionIDs[sessions[i].SessionID] {
			sessions[i].Active = true
		}
		if entry, ok := namesBySessionID[sessions[i].SessionID]; ok && entry.name != "" {
			sessions[i].Name = entry.name
		}
	}
}

type claudeSessionFileMeta struct {
	bypassPermissions bool
}

// claudeProjectDirName encodes a filesystem path the way Claude Code does
// when naming subdirectories under ~/.claude/projects/. Every character that
// is not [A-Za-z0-9-] is replaced with a hyphen — so `/`, `_`, `.`, and any
// other separator collapse to `-`. e.g.
//   /home/u/sdm_smartocr_db   -> -home-u-sdm-smartocr-db
//   /home/u/repo/.worktrees/x -> -home-u-repo--worktrees-x
func claudeProjectDirName(projectPath string) string {
	var b strings.Builder
	b.Grow(len(projectPath))
	for _, r := range projectPath {
		switch {
		case r >= 'a' && r <= 'z',
			r >= 'A' && r <= 'Z',
			r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteByte('-')
		}
	}
	return b.String()
}

// readClaudeSessionMeta checks that a conversation JSONL file exists and
// reads session-level metadata from its first line (the permission-mode record).
func readClaudeSessionMeta(claudeDir, projectPath, sessionID string) (claudeSessionFileMeta, bool) {
	dirName := claudeProjectDirName(projectPath)
	jsonlPath := filepath.Join(claudeDir, "projects", dirName, sessionID+".jsonl")

	f, err := os.Open(jsonlPath)
	if err != nil {
		return claudeSessionFileMeta{}, false
	}
	defer f.Close()

	// Read the first line — typically the permission-mode record.
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)

	var meta claudeSessionFileMeta
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var record struct {
			Type           string `json:"type"`
			PermissionMode string `json:"permissionMode"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			break
		}
		if record.PermissionMode == "bypassPermissions" {
			meta.bypassPermissions = true
		}
		// Only need to read the first record
		break
	}

	return meta, true
}

// readClaudeNameFromTranscript scans a session's conversation JSONL for
// the most recent /rename command and returns its argument. The live
// session metadata file (~/.claude/sessions/<pid>.json) is deleted when
// the agent process exits, so the transcript is the only persistent
// source of the user-set name for non-live sessions.
//
// Returns ("", false) when no /rename event is present (so the caller
// can leave Name unchanged). Returns ("", true) for an explicit clear
// (e.g. /rename with empty args), which the caller should treat as
// authoritative.
func readClaudeNameFromTranscript(claudeDir, projectPath, sessionID string) (string, bool) {
	dirName := claudeProjectDirName(projectPath)
	jsonlPath := filepath.Join(claudeDir, "projects", dirName, sessionID+".jsonl")

	f, err := os.Open(jsonlPath)
	if err != nil {
		return "", false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	needle := []byte(`<command-name>/rename</command-name>`)
	var name string
	var found bool

	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.Contains(line, needle) {
			continue
		}
		var record struct {
			Type    string `json:"type"`
			Subtype string `json:"subtype"`
			Content string `json:"content"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}
		if record.Type != "system" || record.Subtype != "local_command" {
			continue
		}
		n, ok := parseRenameArgs(record.Content)
		if !ok {
			continue
		}
		name = n
		found = true
	}
	return name, found
}

// parseRenameArgs extracts the argument from a /rename local_command
// record's content block. The input event embeds the new name in
// <command-args>...</command-args>; the matching stdout event has no
// command-args block and is skipped (returns false).
func parseRenameArgs(content string) (string, bool) {
	if !strings.Contains(content, "<command-name>/rename</command-name>") {
		return "", false
	}
	start := strings.Index(content, "<command-args>")
	if start < 0 {
		return "", false
	}
	start += len("<command-args>")
	end := strings.Index(content[start:], "</command-args>")
	if end < 0 {
		return "", false
	}
	return strings.TrimSpace(content[start : start+end]), true
}

// enrichClaudeSessionDetail reads the conversation JSONL file to extract
// the away_summary (recap) and last-prompt records for verbose display.
func enrichClaudeSessionDetail(s *AgentSession, claudeDir string) {
	dirName := claudeProjectDirName(s.Dir)
	jsonlPath := filepath.Join(claudeDir, "projects", dirName, s.SessionID+".jsonl")

	f, err := os.Open(jsonlPath)
	if err != nil {
		return
	}
	defer f.Close()

	// Scan the file for away_summary and last-prompt records.
	// These are typically near the end, but we scan the whole file to
	// find the latest away_summary (there can be multiple).
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)

	var lastSummary string
	var lastPrompt string

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		// Quick prefix checks to avoid parsing every line
		if !bytes.Contains(line, []byte(`"type"`)) {
			continue
		}

		var record struct {
			Type       string `json:"type"`
			Subtype    string `json:"subtype"`
			Content    string `json:"content"`
			LastPrompt string `json:"lastPrompt"`
		}
		if err := json.Unmarshal(line, &record); err != nil {
			continue
		}

		switch {
		case record.Type == "system" && record.Subtype == "away_summary" && record.Content != "":
			lastSummary = record.Content
		case record.Type == "last-prompt" && record.LastPrompt != "":
			lastPrompt = record.LastPrompt
		}
	}

	s.Summary = lastSummary
	s.LastPrompt = lastPrompt
}

func isProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	err = process.Signal(syscall.Signal(0))
	return err == nil
}

