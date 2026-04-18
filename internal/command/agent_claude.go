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

	// Enrich with active-process information from session metadata files
	enrichClaudeActiveSessions(sessions, home)

	return sessions
}

// enrichClaudeActiveSessions checks ~/.claude/sessions/*.json to mark
// sessions that are currently running (PID is alive).
func enrichClaudeActiveSessions(sessions []AgentSession, home string) {
	sessionsDir := filepath.Join(home, ".claude", "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return
	}

	// Build a map of sessionId → live PIDs from the metadata files
	type liveMeta struct {
		pid  int
		name string
	}
	liveBySessionID := make(map[string]liveMeta)

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
			liveBySessionID[meta.SessionID] = liveMeta{pid: meta.PID, name: meta.Name}
		}
	}

	if len(liveBySessionID) == 0 {
		return
	}

	for i := range sessions {
		if _, ok := liveBySessionID[sessions[i].SessionID]; ok {
			sessions[i].Active = true
		}
	}
}

type claudeSessionFileMeta struct {
	bypassPermissions bool
}

// readClaudeSessionMeta checks that a conversation JSONL file exists and
// reads session-level metadata from its first line (the permission-mode record).
func readClaudeSessionMeta(claudeDir, projectPath, sessionID string) (claudeSessionFileMeta, bool) {
	dirName := strings.ReplaceAll(projectPath, string(filepath.Separator), "-")
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

// enrichClaudeSessionDetail reads the conversation JSONL file to extract
// the away_summary (recap) and last-prompt records for verbose display.
func enrichClaudeSessionDetail(s *AgentSession, claudeDir string) {
	dirName := strings.ReplaceAll(s.Dir, string(filepath.Separator), "-")
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

