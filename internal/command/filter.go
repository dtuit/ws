package command

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

const (
	activeFilterToken         = "active"
	dirtyFilterToken          = "dirty"
	mineFilterToken           = "mine"
	defaultRecentFilterWindow = 14 * 24 * time.Hour
)

type activityFilterMode int

const (
	activityFilterActive activityFilterMode = iota
	activityFilterDirty
	activityFilterMine
)

type activityFilterSpec struct {
	token        string
	mode         activityFilterMode
	recentWindow time.Duration
}

func resolveCommandRepos(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) ([]manifest.RepoInfo, error) {
	repos, err := resolveFilterRepos(m, wsHome, filter, false)
	if err != nil {
		return nil, err
	}
	if includeWorktrees {
		repos = expandSelectedReposToWorktrees(repos)
	}
	return repos, nil
}

func resolveContextRepos(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) ([]manifest.RepoInfo, error) {
	repos, err := resolveFilterRepos(m, wsHome, filter, true)
	if err != nil {
		return nil, err
	}
	if filter == "" || filter == "all" {
		repos = clonedRepos(repos)
	}
	if includeWorktrees {
		repos = expandSelectedReposToWorktrees(repos)
	}
	return repos, nil
}

func resolveFilterRepos(m *manifest.Manifest, wsHome, filter string, strict bool) ([]manifest.RepoInfo, error) {
	active := m.ActiveRepos()
	repoGroups := m.RepoGroups()

	if filter == manifest.EmptyFilter {
		return nil, nil
	}
	if filter == "" || filter == "all" {
		return m.AllRepos(wsHome), nil
	}

	seen := make(map[string]bool)
	result := make([]manifest.RepoInfo, 0, len(active))
	add := func(repo manifest.RepoInfo) {
		if seen[repo.Name] {
			return
		}
		seen[repo.Name] = true
		result = append(result, repo)
	}

	for _, token := range strings.Split(filter, ",") {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		if spec, ok, err := parseActivityFilterToken(token); ok {
			if err != nil {
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}

			activityRepos, err := resolveActivityRepos(m, wsHome, spec)
			if err != nil {
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			for _, repo := range activityRepos {
				add(repo)
			}
			continue
		}

		if members, ok := m.Groups[token]; ok {
			for _, name := range members {
				cfg, ok := active[name]
				if ok {
					add(baseRepoInfo(m, wsHome, name, cfg, repoGroups[name]))
					continue
				}
				// Try as a worktree token (e.g., "repo@feature")
				repoName, selector, isWT := splitWorktreeToken(name, active)
				if !isWT || selector == "" {
					continue
				}
				wtCfg := active[repoName]
				target, err := resolveExplicitWorktreeTarget(baseRepoInfo(m, wsHome, repoName, wtCfg, repoGroups[repoName]), selector)
				if err != nil {
					if strict {
						return nil, fmt.Errorf("group %q member %q: %w", token, name, err)
					}
					fmt.Fprintf(os.Stderr, "Warning: group %q member %q: %v\n", token, name, err)
					continue
				}
				add(target)
			}
			continue
		}

		if cfg, ok := active[token]; ok {
			add(baseRepoInfo(m, wsHome, token, cfg, repoGroups[token]))
			continue
		}

		repoName, selector, ok := splitWorktreeToken(token, active)
		if ok {
			if selector == "" {
				err := fmt.Errorf("worktree target %q is missing a worktree name", token)
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}

			cfg := active[repoName]
			target, err := resolveExplicitWorktreeTarget(baseRepoInfo(m, wsHome, repoName, cfg, repoGroups[repoName]), selector)
			if err != nil {
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			add(target)
			continue
		}

		// @branch — select all worktrees across repos matching a branch name
		if strings.HasPrefix(token, "@") {
			branch := token[1:]
			if branch == "" {
				err := fmt.Errorf("@ requires a branch name (e.g., @feature-a)")
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
				continue
			}
			matched, err := resolveWorktreeBranch(m, wsHome, branch)
			if err != nil {
				if strict {
					return nil, err
				}
				fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
			}
			for _, repo := range matched {
				add(repo)
			}
			continue
		}

		err := fmt.Errorf("%q is not a known group, repo, or worktree target", token)
		if strict {
			return nil, err
		}
		fmt.Fprintf(os.Stderr, "Warning: %v\n", err)
	}

	return result, nil
}

func parseActivityFilterToken(token string) (activityFilterSpec, bool, error) {
	switch token {
	case activeFilterToken:
		return activityFilterSpec{
			token:        token,
			mode:         activityFilterActive,
			recentWindow: defaultRecentFilterWindow,
		}, true, nil
	case dirtyFilterToken:
		return activityFilterSpec{
			token: token,
			mode:  activityFilterDirty,
		}, true, nil
	case mineFilterToken:
		return activityFilterSpec{}, true, fmt.Errorf("%q requires a duration, e.g. %s:1d", mineFilterToken, mineFilterToken)
	}

	prefix, raw, ok := strings.Cut(token, ":")
	if !ok {
		return activityFilterSpec{}, false, nil
	}

	switch prefix {
	case activeFilterToken, mineFilterToken, dirtyFilterToken:
	default:
		return activityFilterSpec{}, false, nil
	}

	if raw == "" {
		return activityFilterSpec{}, true, fmt.Errorf("%q requires a duration", token)
	}
	if prefix == dirtyFilterToken {
		return activityFilterSpec{}, true, fmt.Errorf("%q does not accept a duration", dirtyFilterToken)
	}

	recentWindow, err := parseRecentFilterWindow(raw)
	if err != nil {
		return activityFilterSpec{}, true, fmt.Errorf("%q uses an invalid duration: %w", token, err)
	}

	spec := activityFilterSpec{
		token:        token,
		recentWindow: recentWindow,
	}
	if prefix == activeFilterToken {
		spec.mode = activityFilterActive
	} else {
		spec.mode = activityFilterMine
	}
	return spec, true, nil
}

func parseRecentFilterWindow(raw string) (time.Duration, error) {
	raw = strings.TrimSpace(strings.ToLower(raw))
	if raw == "" {
		return 0, fmt.Errorf("duration is required")
	}

	if len(raw) < 2 {
		return 0, fmt.Errorf("unsupported duration %q", raw)
	}

	value, err := strconv.ParseInt(raw[:len(raw)-1], 10, 64)
	if err != nil || value <= 0 {
		return 0, fmt.Errorf("unsupported duration %q", raw)
	}

	var unit time.Duration
	switch raw[len(raw)-1] {
	case 's':
		unit = time.Second
	case 'm':
		unit = time.Minute
	case 'h':
		unit = time.Hour
	case 'd':
		unit = 24 * time.Hour
	case 'w':
		unit = 7 * 24 * time.Hour
	default:
		return 0, fmt.Errorf("unsupported duration %q", raw)
	}

	return time.Duration(value) * unit, nil
}

func resolveActivityRepos(m *manifest.Manifest, wsHome string, spec activityFilterSpec) ([]manifest.RepoInfo, error) {
	candidates := m.AllRepos(wsHome)
	if len(candidates) == 0 {
		return nil, nil
	}

	activity := git.InspectRepoActivityAll(candidates, spec.recentWindow, git.Workers(len(candidates)))
	matched := make([]manifest.RepoInfo, 0, len(candidates))
	var failures []string

	for i, state := range activity {
		switch {
		case state.Err == nil:
			if matchesActivityFilter(spec, state) {
				matched = append(matched, candidates[i])
			}
		case errors.Is(state.Err, git.ErrNotCloned):
			continue
		default:
			failures = append(failures, fmt.Sprintf("%s: %v", candidates[i].Name, state.Err))
		}
	}

	if len(failures) > 0 {
		return matched, fmt.Errorf("%s filter failed for %s", spec.token, strings.Join(failures, "; "))
	}

	return matched, nil
}

func matchesActivityFilter(spec activityFilterSpec, state git.RepoActivity) bool {
	switch spec.mode {
	case activityFilterDirty:
		return state.Dirty
	case activityFilterMine:
		return state.RecentLocalCommit
	default:
		return state.Dirty || state.RecentLocalCommit
	}
}

func baseRepoInfo(m *manifest.Manifest, wsHome, name string, cfg manifest.RepoConfig, groups []string) manifest.RepoInfo {
	return manifest.RepoInfo{
		Name:   name,
		URL:    m.ResolveURL(name, cfg),
		Branch: m.ResolveBranch(cfg),
		Groups: groups,
		Path:   m.ResolvePath(wsHome, name, cfg),
	}
}

// resolveWorktreeBranch finds all worktrees across active repos that are on
// the given branch. Returns one RepoInfo per matching worktree.
func resolveWorktreeBranch(m *manifest.Manifest, wsHome, branch string) ([]manifest.RepoInfo, error) {
	allRepos := m.AllRepos(wsHome)
	if len(allRepos) == 0 {
		return nil, nil
	}

	sets := git.DiscoverWorktreesAll(allRepos, git.Workers(len(allRepos)))
	var matched []manifest.RepoInfo

	for _, set := range sets {
		if set.Err != nil || len(set.Worktrees) == 0 {
			continue
		}
		for _, target := range worktreeTargets(set.Repo, set.Worktrees) {
			if target.Primary {
				continue
			}
			if target.Branch == branch {
				matched = append(matched, manifest.RepoInfo{
					Name:     target.Name,
					URL:      set.Repo.URL,
					Branch:   target.Branch,
					Groups:   set.Repo.Groups,
					Path:     target.Path,
					Worktree: worktreeDisplayName(set.Repo.Name, target.Name),
				})
			}
		}
	}

	return matched, nil
}

func expandSelectedReposToWorktrees(repos []manifest.RepoInfo) []manifest.RepoInfo {
	if len(repos) == 0 {
		return nil
	}

	baseRepos := make([]manifest.RepoInfo, 0, len(repos))
	for _, repo := range repos {
		if repo.Worktree != "" {
			continue
		}
		baseRepos = append(baseRepos, repo)
	}

	expandedByName := make(map[string][]manifest.RepoInfo, len(baseRepos))
	for _, set := range expandReposToWorktreeSets(baseRepos) {
		expandedByName[set.base.Name] = set.expanded
	}

	seen := make(map[string]bool)
	expanded := make([]manifest.RepoInfo, 0, len(repos))
	add := func(repo manifest.RepoInfo) {
		if seen[repo.Name] {
			return
		}
		seen[repo.Name] = true
		expanded = append(expanded, repo)
	}

	for _, repo := range repos {
		if repo.Worktree != "" {
			add(repo)
			continue
		}
		for _, target := range expandedByName[repo.Name] {
			add(target)
		}
	}

	return expanded
}

type worktreeExpansion struct {
	base     manifest.RepoInfo
	expanded []manifest.RepoInfo
}

func expandReposToWorktreeSets(repos []manifest.RepoInfo) []worktreeExpansion {
	sets := git.DiscoverWorktreesAll(repos, git.Workers(len(repos)))
	expanded := make([]worktreeExpansion, 0, len(sets))

	for _, set := range sets {
		if set.Err != nil || len(set.Worktrees) == 0 {
			expanded = append(expanded, worktreeExpansion{
				base:     set.Repo,
				expanded: []manifest.RepoInfo{set.Repo},
			})
			continue
		}

		reposForBase := make([]manifest.RepoInfo, 0, len(set.Worktrees))
		for _, target := range worktreeTargets(set.Repo, set.Worktrees) {
			worktreeName := ""
			if !target.Primary {
				worktreeName = worktreeDisplayName(set.Repo.Name, target.Name)
			}
			reposForBase = append(reposForBase, manifest.RepoInfo{
				Name:     target.Name,
				URL:      set.Repo.URL,
				Branch:   target.Branch,
				Groups:   set.Repo.Groups,
				Path:     target.Path,
				Worktree: worktreeName,
			})
		}
		expanded = append(expanded, worktreeExpansion{
			base:     set.Repo,
			expanded: reposForBase,
		})
	}

	return expanded
}
