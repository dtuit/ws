package command

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// WorktreeAdd creates a worktree for the given branch in each repo matching
// the filter (or current context). If the branch doesn't exist locally, it is
// created from HEAD.
func WorktreeAdd(m *manifest.Manifest, wsHome, branch, filter string) error {
	repos, err := resolveWorktreeRepos(m, wsHome, filter)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	wtRoot := m.ResolveWorktreeRoot(wsHome)
	sanitized := sanitizeWorktreeKey(branch)
	created := 0
	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			continue
		}

		wtPath := filepath.Join(wtRoot, repo.Name, sanitized)
		if _, err := os.Stat(wtPath); err == nil {
			fmt.Printf("  %s: worktree already exists at %s\n", repo.Name, wtPath)
			continue
		}

		if err := os.MkdirAll(filepath.Dir(wtPath), 0755); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", repo.Name, err)
			continue
		}

		// Try to add with existing branch first, fall back to creating new branch.
		if err := gitWorktreeAdd(repo.Path, wtPath, branch); err != nil {
			fmt.Fprintf(os.Stderr, "  %s: %v\n", repo.Name, err)
			continue
		}
		fmt.Printf("  %s: created worktree at %s\n", repo.Name, filepath.Base(wtPath))
		created++
	}

	if created > 0 {
		fmt.Printf("Created %d worktree(s) on branch %q.\n", created, branch)
	} else {
		fmt.Println("No worktrees created.")
	}
	return nil
}

// WorktreeRemove removes the worktree for the given branch from each repo
// matching the filter (or current context).
func WorktreeRemove(m *manifest.Manifest, wsHome, branch, filter string) error {
	repos, err := resolveWorktreeRepos(m, wsHome, filter)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	removed := 0
	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			continue
		}

		worktrees, err := git.DiscoverWorktrees(repo)
		if err != nil {
			continue
		}

		for _, wt := range worktrees {
			if wt.Primary {
				continue
			}
			if wt.Branch == branch || filepath.Base(wt.Path) == repo.Name+"-"+sanitizeWorktreeKey(branch) {
				if err := gitWorktreeRemove(repo.Path, wt.Path); err != nil {
					fmt.Fprintf(os.Stderr, "  %s: %v\n", repo.Name, err)
					continue
				}
				fmt.Printf("  %s: removed worktree %s\n", repo.Name, filepath.Base(wt.Path))
				removed++
				break
			}
		}
	}

	if removed > 0 {
		fmt.Printf("Removed %d worktree(s).\n", removed)
	} else {
		fmt.Println("No matching worktrees found.")
	}
	return nil
}

// WorktreeListCmd lists worktrees for each repo matching the filter.
func WorktreeListCmd(m *manifest.Manifest, wsHome, filter string) error {
	repos, err := resolveWorktreeRepos(m, wsHome, filter)
	if err != nil {
		return err
	}
	if len(repos) == 0 {
		fmt.Println("No repos matched the filter.")
		return nil
	}

	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			continue
		}

		worktrees, err := git.DiscoverWorktrees(repo)
		if err != nil {
			continue
		}

		linked := 0
		for _, wt := range worktrees {
			if !wt.Primary {
				linked++
			}
		}
		if linked == 0 {
			continue
		}

		fmt.Printf("%s:\n", repo.Name)
		for _, target := range worktreeTargets(repo, worktrees) {
			if target.Primary {
				continue
			}
			displayName := worktreeDisplayName(repo.Name, target.Name)
			fmt.Printf("  %s  (%s)  %s\n", displayName, target.Branch, target.Path)
		}
	}
	return nil
}

func resolveWorktreeRepos(m *manifest.Manifest, wsHome, filter string) ([]manifest.RepoInfo, error) {
	if filter != "" {
		return resolveFilterRepos(m, wsHome, filter, false)
	}
	rawCtx := GetContext(wsHome)
	if rawCtx != "" {
		return resolveFilterRepos(m, wsHome, rawCtx, false)
	}
	return m.AllRepos(wsHome), nil
}

func gitWorktreeAdd(repoDir, wtPath, branch string) error {
	// Try with existing branch first
	cmd := exec.Command("git", "worktree", "add", wtPath, branch)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err == nil {
		return nil
	} else if !strings.Contains(string(out), "not a valid reference") &&
		!strings.Contains(string(out), "invalid reference") {
		return fmt.Errorf("git worktree add: %s", strings.TrimSpace(string(out)))
	}

	// Branch doesn't exist — create it
	cmd = exec.Command("git", "worktree", "add", "-b", branch, wtPath)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree add -b: %s", strings.TrimSpace(string(out)))
	}
	return nil
}

func gitWorktreeRemove(repoDir, wtPath string) error {
	cmd := exec.Command("git", "worktree", "remove", wtPath)
	cmd.Dir = repoDir
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("git worktree remove: %s", strings.TrimSpace(string(out)))
	}
	return nil
}
