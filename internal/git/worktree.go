package git

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/dtuit/ws/internal/manifest"
)

// ErrNotCloned indicates a manifest repo path is not a git checkout on disk.
var ErrNotCloned = errors.New("not cloned")

// WorktreeInfo describes one checkout attached to a git common dir.
type WorktreeInfo struct {
	Path      string
	Branch    string
	Head      string
	Primary   bool
	CommonDir string
}

// WorktreeSet is the discovered worktree state for one manifest repo.
type WorktreeSet struct {
	Repo      manifest.RepoInfo
	Worktrees []WorktreeInfo
	Err       error
}

// IsCheckout reports whether the path is a git checkout or linked worktree.
func IsCheckout(path string) bool {
	_, err := GitDir(path)
	return err == nil
}

// GitDir returns the absolute git dir for a checkout path.
func GitDir(repoDir string) (string, error) {
	out, err := gitCmd(repoDir, "rev-parse", "--git-dir")
	if err != nil {
		return "", ErrNotCloned
	}
	return resolveGitPath(repoDir, out), nil
}

// GitCommonDir returns the absolute common git dir shared by all worktrees.
func GitCommonDir(repoDir string) (string, error) {
	out, err := gitCmd(repoDir, "rev-parse", "--git-common-dir")
	if err != nil {
		return "", ErrNotCloned
	}
	return resolveGitPath(repoDir, out), nil
}

// HasStash reports whether the repository has any stash entries.
func HasStash(repoDir string) (bool, error) {
	commonDir, err := GitCommonDir(repoDir)
	if err != nil {
		return false, err
	}
	stashPath := filepath.Join(commonDir, "logs", "refs", "stash")
	if _, err := os.Stat(stashPath); err == nil {
		return true, nil
	} else if os.IsNotExist(err) {
		return false, nil
	} else {
		return false, err
	}
}

// DiscoverWorktrees returns all linked worktrees attached to a manifest repo.
// The manifest repo path is treated as the primary checkout for ws.
func DiscoverWorktrees(repo manifest.RepoInfo) ([]WorktreeInfo, error) {
	if !IsCheckout(repo.Path) {
		return nil, ErrNotCloned
	}

	commonDir, err := GitCommonDir(repo.Path)
	if err != nil {
		return nil, err
	}

	out, err := gitCmd(repo.Path, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}

	worktrees, err := parseWorktreeList(out)
	if err != nil {
		return nil, err
	}
	if len(worktrees) == 0 {
		worktrees = []WorktreeInfo{{Path: filepath.Clean(repo.Path)}}
	}

	primaryPath := filepath.Clean(repo.Path)
	for i := range worktrees {
		worktrees[i].Path = filepath.Clean(worktrees[i].Path)
		worktrees[i].Primary = worktrees[i].Path == primaryPath
		worktrees[i].CommonDir = commonDir
		if worktrees[i].Branch == "" {
			worktrees[i].Branch = repo.Branch
		}
	}

	sort.Slice(worktrees, func(i, j int) bool {
		if worktrees[i].Primary != worktrees[j].Primary {
			return worktrees[i].Primary
		}
		return worktrees[i].Path < worktrees[j].Path
	})

	return worktrees, nil
}

// DiscoverWorktreesAll resolves worktrees for multiple repos in parallel.
func DiscoverWorktreesAll(repos []manifest.RepoInfo, maxWorkers int) []WorktreeSet {
	results := make([]WorktreeSet, len(repos))
	if len(repos) == 0 {
		return results
	}
	if maxWorkers < 1 {
		maxWorkers = 1
	}

	sem := make(chan struct{}, maxWorkers)
	var wg sync.WaitGroup

	for i, repo := range repos {
		wg.Add(1)
		go func(idx int, r manifest.RepoInfo) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			worktrees, err := DiscoverWorktrees(r)
			results[idx] = WorktreeSet{
				Repo:      r,
				Worktrees: worktrees,
				Err:       err,
			}
		}(i, repo)
	}

	wg.Wait()
	return results
}

func resolveGitPath(repoDir, gitPath string) string {
	gitPath = strings.TrimSpace(gitPath)
	if filepath.IsAbs(gitPath) {
		return filepath.Clean(gitPath)
	}
	return filepath.Clean(filepath.Join(repoDir, gitPath))
}

func parseWorktreeList(output string) ([]WorktreeInfo, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, nil
	}

	lines := strings.Split(output, "\n")
	result := make([]WorktreeInfo, 0, len(lines))
	var current *WorktreeInfo

	flush := func() error {
		if current == nil {
			return nil
		}
		if current.Path == "" {
			return fmt.Errorf("invalid git worktree output: missing worktree path")
		}
		result = append(result, *current)
		current = nil
		return nil
	}

	for _, line := range lines {
		if line == "" {
			if err := flush(); err != nil {
				return nil, err
			}
			continue
		}

		if current == nil {
			current = &WorktreeInfo{}
		}

		switch {
		case strings.HasPrefix(line, "worktree "):
			current.Path = strings.TrimPrefix(line, "worktree ")
		case strings.HasPrefix(line, "HEAD "):
			current.Head = strings.TrimPrefix(line, "HEAD ")
		case strings.HasPrefix(line, "branch "):
			current.Branch = shortBranch(strings.TrimPrefix(line, "branch "))
		case line == "detached":
			current.Branch = "(detached)"
		}
	}

	if err := flush(); err != nil {
		return nil, err
	}
	return result, nil
}

func shortBranch(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}
