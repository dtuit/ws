package command

import (
	"errors"
	"fmt"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// List prints a table of repos from the manifest with their status.
func List(m *manifest.Manifest, wsHome string, showAll, includeWorktrees bool) error {
	repos := m.AllRepos(wsHome)
	if includeWorktrees {
		return listWorktrees(m, repos, showAll)
	}

	repoGroups := m.RepoGroups()
	worktreeSets := git.DiscoverWorktreesAll(repos, git.Workers(len(repos)))
	worktreeCounts := make(map[string]int, len(worktreeSets))
	clonedState := make(map[string]string, len(worktreeSets))

	for _, set := range worktreeSets {
		switch {
		case set.Err == nil:
			worktreeCounts[set.Repo.Name] = len(set.Worktrees)
			clonedState[set.Repo.Name] = "yes"
		case errors.Is(set.Err, git.ErrNotCloned):
			clonedState[set.Repo.Name] = "-"
		default:
			worktreeCounts[set.Repo.Name] = 1
			clonedState[set.Repo.Name] = "yes"
		}
	}

	fmt.Printf("%-42s %-16s %-10s %-3s %s\n", "REPO", "BRANCH", "GROUPS", "WT", "CLONED")
	fmt.Println(strings.Repeat("-", 84))

	for _, r := range repos {
		groups := strings.Join(repoGroups[r.Name], ",")
		if groups == "" {
			groups = "-"
		}
		fmt.Printf("%-42s %-16s %-10s %-3d %s\n",
			r.Name, r.Branch, groups, worktreeCounts[r.Name], clonedState[r.Name])
	}

	if showAll && len(m.Exclude) > 0 {
		fmt.Printf("\n%-42s %s\n", "EXCLUDED", "")
		fmt.Println(strings.Repeat("-", 78))
		for _, name := range m.Exclude {
			fmt.Printf("%-42s (see manifest.yml)\n", name)
		}
	}

	return nil
}

func listWorktrees(m *manifest.Manifest, repos []manifest.RepoInfo, showAll bool) error {
	fmt.Printf("%-24s %-14s %-18s %-6s %s\n", "REPO", "CHECKOUT", "BRANCH", "CLONED", "PATH")
	fmt.Println(strings.Repeat("-", 96))

	worktreeSets := git.DiscoverWorktreesAll(repos, git.Workers(len(repos)))
	for _, set := range worktreeSets {
		switch {
		case set.Err == nil:
			for _, target := range worktreeTargets(set.Repo, set.Worktrees) {
				checkout := "manifest"
				if !target.Primary {
					checkout = worktreeDisplayName(set.Repo.Name, target.Name)
				}
				fmt.Printf("%-24s %-14s %-18s %-6s %s\n",
					set.Repo.Name, checkout, target.Branch, "yes", target.Path)
			}
		case errors.Is(set.Err, git.ErrNotCloned):
			fmt.Printf("%-24s %-14s %-18s %-6s %s\n",
				set.Repo.Name, "manifest", set.Repo.Branch, "no", set.Repo.Path)
		default:
			fmt.Printf("%-24s %-14s %-18s %-6s %s\n",
				set.Repo.Name, "manifest", set.Repo.Branch, "error", set.Repo.Path)
		}
	}

	if showAll && len(m.Exclude) > 0 {
		fmt.Printf("\n%-24s %s\n", "EXCLUDED", "")
		fmt.Println(strings.Repeat("-", 96))
		for _, name := range m.Exclude {
			fmt.Printf("%-24s (see manifest.yml)\n", name)
		}
	}

	return nil
}
