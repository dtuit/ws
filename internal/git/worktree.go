package git

import (
	"bytes"
	"os/exec"
	"path/filepath"
	"strings"
)

// WorktreePaths returns all worktree paths for the git repository containing repoDir.
// The result includes repoDir itself when it is part of the worktree set.
func WorktreePaths(repoDir string) ([]string, error) {
	cmd := exec.Command("git", "worktree", "list", "--porcelain")
	cmd.Dir = repoDir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, err
	}

	var paths []string
	seen := make(map[string]bool)
	for _, line := range strings.Split(stdout.String(), "\n") {
		if !strings.HasPrefix(line, "worktree ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(line, "worktree "))
		if path == "" {
			continue
		}
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		paths = append(paths, path)
	}

	return paths, nil
}
