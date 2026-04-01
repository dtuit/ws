package command

import (
	"fmt"
	"os/exec"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Super runs an arbitrary command in each repo directory.
func Super(m *manifest.Manifest, wsHome, filter string, cmdArgs []string, includeWorktrees bool) error {
	// Validate the command exists before fanning out
	if _, err := exec.LookPath(cmdArgs[0]); err != nil {
		return fmt.Errorf("command not found: %s", cmdArgs[0])
	}

	repos := m.ResolveFilter(filter, wsHome)
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}
	if includeWorktrees {
		repos = expandReposToWorktrees(repos)
	}

	workers := git.Workers(len(repos))
	failCount := git.Exec(repos, cmdArgs, workers)
	if failCount > 0 {
		return fmt.Errorf("%d repo(s) failed", failCount)
	}
	return nil
}

// ParseSuperArgs disambiguates the filter and command arguments.
// Leading ws flags are parsed before the command starts.
func ParseSuperArgs(m *manifest.Manifest, args []string) (filter string, cmdArgs []string, includeWorktrees bool) {
	for i, arg := range args {
		switch {
		case arg == "--worktrees" || arg == "-W":
			includeWorktrees = true
		case filter == "" && isFilterToken(m, arg):
			filter = arg
		default:
			return filter, args[i:], includeWorktrees
		}
	}
	return filter, nil, includeWorktrees
}
