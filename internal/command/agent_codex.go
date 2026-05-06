package command

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// discoverCodexSessions queries ~/.codex/state_5.sqlite for threads
// whose cwd matches the workspace path index. When external is true,
// returns sessions whose cwd does NOT match.
func discoverCodexSessions(pathIndex map[string]string, external bool) []AgentSession {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}

	dbPath := filepath.Join(home, ".codex", "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}

	sqlite3, err := exec.LookPath("sqlite3")
	if err != nil {
		return nil
	}

	query := `SELECT id, cwd, first_user_message, model, created_at, updated_at, title FROM threads ORDER BY updated_at DESC`
	cmd := exec.Command(sqlite3, "-separator", "\t", dbPath, query)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return nil
	}

	var sessions []AgentSession
	for _, line := range strings.Split(out.String(), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		fields := strings.SplitN(line, "\t", 7)
		if len(fields) < 7 {
			continue
		}

		id := fields[0]
		cwd := fields[1]
		prompt := fields[2]
		model := fields[3]
		createdAt := parseUnixSeconds(fields[4])
		updatedAt := parseUnixSeconds(fields[5])
		title := fields[6]

		repo, matched := matchSessionRepo(cwd, pathIndex)
		if external {
			if matched {
				continue
			}
			repo = externalRepoLabel(cwd)
		} else if !matched {
			continue
		}

		sessions = append(sessions, AgentSession{
			Agent:      agentCodex,
			SessionID:  id,
			Repo:       repo,
			Dir:        cwd,
			StartedAt:  createdAt,
			LastActive: updatedAt,
			Prompt:     prompt,
			Model:      model,
			Name:       codexThreadName(title, prompt),
		})
	}

	return sessions
}

// codexThreadName returns the user-set thread name, or "" when the
// thread is unnamed. Codex pre-populates the threads.title column with
// the first user message, so we treat title as a name only when it
// differs from first_user_message — that signals an explicit rename.
func codexThreadName(title, firstUserMessage string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return ""
	}
	if title == strings.TrimSpace(firstUserMessage) {
		return ""
	}
	return title
}

func parseUnixSeconds(s string) time.Time {
	sec, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return time.Time{}
	}
	return time.Unix(sec, 0)
}
