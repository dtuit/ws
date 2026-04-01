package command

import (
	"fmt"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Pull runs git pull --ff-only across repos with progress and per-repo output.
func Pull(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) error {
	repos := m.ResolveFilter(filter, wsHome)
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}
	if includeWorktrees {
		repos = expandReposToWorktrees(repos)
	}

	workers := git.Workers(len(repos))
	failCount := git.RunAll(repos, []string{"git", "pull", "--ff-only"}, workers, git.RunOpts{
		Verb:     "pulling",
		Summary:  "Pulled",
		Suppress: "Already up to date.",
	})
	if failCount > 0 {
		return fmt.Errorf("%d repo(s) failed", failCount)
	}
	return nil
}
