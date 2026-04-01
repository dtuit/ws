package command

import (
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

type worktreeTarget struct {
	RepoName string
	Name     string
	Branch   string
	Path     string
	Primary  bool
}

func expandReposToWorktrees(repos []manifest.RepoInfo) []manifest.RepoInfo {
	sets := git.DiscoverWorktreesAll(repos, git.Workers(len(repos)))
	expanded := make([]manifest.RepoInfo, 0, len(repos))

	for _, set := range sets {
		if set.Err != nil || len(set.Worktrees) == 0 {
			expanded = append(expanded, set.Repo)
			continue
		}
		for _, target := range worktreeTargets(set.Repo, set.Worktrees) {
			expanded = append(expanded, manifest.RepoInfo{
				Name:   target.Name,
				URL:    set.Repo.URL,
				Branch: target.Branch,
				Groups: set.Repo.Groups,
				Path:   target.Path,
			})
		}
	}

	return expanded
}

func worktreeTargets(repo manifest.RepoInfo, worktrees []git.WorktreeInfo) []worktreeTarget {
	if len(worktrees) == 0 {
		return []worktreeTarget{{
			RepoName: repo.Name,
			Name:     repo.Name,
			Branch:   repo.Branch,
			Path:     repo.Path,
			Primary:  true,
		}}
	}

	keys := make([]string, len(worktrees))
	counts := make(map[string]int)
	for i, wt := range worktrees {
		if wt.Primary {
			continue
		}
		key := worktreeKey(repo.Name, wt)
		keys[i] = key
		counts[key]++
	}

	used := make(map[string]int)
	targets := make([]worktreeTarget, 0, len(worktrees))
	for i, wt := range worktrees {
		name := repo.Name
		if !wt.Primary {
			key := keys[i]
			used[key]++
			if counts[key] > 1 {
				key = fmt.Sprintf("%s#%d", key, used[key])
			}
			name = repo.Name + "@" + key
		}

		branch := wt.Branch
		if branch == "" {
			branch = repo.Branch
		}

		targets = append(targets, worktreeTarget{
			RepoName: repo.Name,
			Name:     name,
			Branch:   branch,
			Path:     wt.Path,
			Primary:  wt.Primary,
		})
	}

	sort.Slice(targets, func(i, j int) bool {
		if targets[i].Primary != targets[j].Primary {
			return targets[i].Primary
		}
		return targets[i].Name < targets[j].Name
	})

	return targets
}

func worktreeDisplayName(repoName, targetName string) string {
	prefix := repoName + "@"
	return strings.TrimPrefix(targetName, prefix)
}

func worktreeKey(repoName string, wt git.WorktreeInfo) string {
	key := filepath.Base(wt.Path)
	if key == "." || key == string(filepath.Separator) || key == "" {
		key = "worktree"
	}
	if key == repoName && wt.Branch != "" && wt.Branch != "(detached)" {
		key = wt.Branch
	}
	return sanitizeWorktreeKey(key)
}

func sanitizeWorktreeKey(key string) string {
	replacer := strings.NewReplacer(
		"/", "-",
		"\\", "-",
		" ", "-",
		":", "-",
	)
	key = replacer.Replace(key)
	key = strings.Trim(key, "-")
	if key == "" {
		return "worktree"
	}
	return key
}

// CDPath resolves the path for `ws cd`, optionally selecting a discovered worktree.
func CDPath(repo manifest.RepoInfo, selector string) (string, error) {
	if selector == "" || selector == "manifest" || selector == "primary" {
		return repo.Path, nil
	}

	worktrees, err := git.DiscoverWorktrees(repo)
	if err != nil {
		if errors.Is(err, git.ErrNotCloned) {
			return "", fmt.Errorf("%s is not cloned", repo.Name)
		}
		return "", err
	}

	type candidate struct {
		branch string
		path   string
	}

	var matches []candidate
	cleanSelector := filepath.Clean(selector)
	for _, wt := range worktrees {
		if wt.Primary {
			continue
		}
		if filepath.Clean(wt.Path) == cleanSelector || filepath.Base(wt.Path) == selector || wt.Branch == selector {
			matches = append(matches, candidate{branch: wt.Branch, path: wt.Path})
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no worktree %q found for %s", selector, repo.Name)
	case 1:
		return matches[0].path, nil
	default:
		lines := make([]string, 0, len(matches))
		for _, match := range matches {
			if match.branch != "" {
				lines = append(lines, fmt.Sprintf("%s (%s)", match.path, match.branch))
			} else {
				lines = append(lines, match.path)
			}
		}
		sort.Strings(lines)
		return "", fmt.Errorf("worktree %q for %s is ambiguous:\n  %s", selector, repo.Name, strings.Join(lines, "\n  "))
	}
}
