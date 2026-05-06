package command

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
	"gopkg.in/yaml.v3"
)

// SetContext sets the default filter, regenerates the VS Code workspace,
// and updates the workspace scope hint file to match.
func SetContext(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) error {
	return applyContext(m, wsHome, activeWorkspaceName(), filter, includeWorktrees, applyModeSet)
}

// RefreshContext re-resolves the stored context filter and rebuilds the scope hint.
func RefreshContext(m *manifest.Manifest, wsHome string, includeWorktrees bool) error {
	workspace := activeWorkspaceName()
	state, ok, err := loadWorkspaceContextState(wsHome, workspace)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("no context set")
	}

	return applyContext(m, wsHome, workspace, state.Raw, includeWorktrees, applyModeRefresh)
}

// SwapContext swaps the current context with the previously set one,
// analogous to `cd -`. Returns an error if no previous context is recorded.
func SwapContext(m *manifest.Manifest, wsHome string, includeWorktrees bool) error {
	workspace := activeWorkspaceName()
	state, ok, err := loadWorkspaceContextState(wsHome, workspace)
	if err != nil {
		return err
	}
	if !ok || state.Previous == nil {
		return fmt.Errorf("no previous context")
	}

	return applyContext(m, wsHome, workspace, *state.Previous, includeWorktrees, applyModeSwap)
}

type applyMode int

const (
	applyModeSet applyMode = iota
	applyModeRefresh
	applyModeSwap
)

func applyContext(m *manifest.Manifest, wsHome, workspace, filter string, includeWorktrees bool, mode applyMode) error {
	filter, err := normalizeContextFilter(m, filter)
	if err != nil {
		return err
	}

	prevState, hasPrev, err := loadWorkspaceContextState(wsHome, workspace)
	if err != nil {
		return err
	}

	// Determine what to store as `previous` on this write.
	// - refresh: never shift; pass through the existing previous.
	// - set/swap: shift when the raw filter actually changes; otherwise
	//   preserve the existing previous so no-op writes don't clobber it.
	var previous *string
	if mode == applyModeRefresh {
		if hasPrev {
			previous = prevState.Previous
		}
	} else if hasPrev && prevState.Raw != filter {
		prior := prevState.Raw
		previous = &prior
	} else if hasPrev {
		previous = prevState.Previous
	}

	repos, err := resolveContextRepos(m, wsHome, filter, includeWorktrees)
	if err != nil {
		return err
	}
	if filter == "" || filter == "none" || filter == "reset" {
		if err := persistContextState(wsHome, workspace, "", repos, previous); err != nil {
			return err
		}
		switch mode {
		case applyModeRefresh:
			fmt.Printf("Context refreshed (no saved filter, %d repos)\n", len(repos))
			fmt.Printf("Resolved: %s\n", formatResolvedContextRepos(repos))
		case applyModeSwap:
			fmt.Printf("Context swapped to cleared (%d repos)\n", len(repos))
			fmt.Printf("Resolved: %s\n", formatResolvedContextRepos(repos))
		default:
			fmt.Println("Context cleared.")
		}
		if err := clearLegacyScopeDirs(m, wsHome); err != nil {
			return err
		}
		if err := syncScopeHint(wsHome, workspace, "", repos); err != nil {
			return err
		}
		return writeWorkspace(m, wsHome, workspace, repos, includeWorktrees)
	}

	if len(repos) == 0 && filter != "all" {
		return fmt.Errorf("filter %q matched no repos", filter)
	}

	if err := persistContextState(wsHome, workspace, filter, repos, previous); err != nil {
		return err
	}
	switch mode {
	case applyModeRefresh:
		fmt.Printf("Context refreshed from %q (%d repos)\n", filter, len(repos))
	case applyModeSwap:
		fmt.Printf("Context swapped to %q (%d repos)\n", filter, len(repos))
	default:
		fmt.Printf("Context set to %q (%d repos)\n", filter, len(repos))
	}
	fmt.Printf("Resolved: %s\n", formatResolvedContextRepos(repos))

	if err := clearLegacyScopeDirs(m, wsHome); err != nil {
		return err
	}
	if err := syncScopeHint(wsHome, workspace, filter, repos); err != nil {
		return err
	}
	return writeWorkspace(m, wsHome, workspace, repos, includeWorktrees)
}

// AddContext extends the current context with more groups or repos.
// If no context is set, it behaves like SetContext.
func AddContext(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) error {
	addition, err := normalizeContextFilter(m, filter)
	if err != nil {
		return err
	}
	if addition == "" {
		return fmt.Errorf("usage: ws context add [%s] <filter>", WorktreesFlagUsage)
	}

	currentRaw := storedContextRaw(wsHome)
	current, err := normalizeContextFilter(m, currentRaw)
	if err != nil {
		return fmt.Errorf("existing context %q is invalid: %w", currentRaw, err)
	}

	merged := mergeContextFilters(current, addition)
	return SetContext(m, wsHome, merged, includeWorktrees)
}

// RemoveContext subtracts groups or repos from the current effective context.
// The remaining scope is persisted as an explicit repo list.
func RemoveContext(m *manifest.Manifest, wsHome, filter string, includeWorktrees bool) error {
	removal, err := normalizeContextFilter(m, filter)
	if err != nil {
		return err
	}
	if removal == "" {
		return fmt.Errorf("usage: ws context remove [%s] <filter>", WorktreesFlagUsage)
	}

	currentFilter, ok := GetDefaultContextForMode(m, wsHome, includeWorktrees)
	if !ok {
		return fmt.Errorf("no context set")
	}

	currentRepos, err := resolveCommandRepos(m, wsHome, currentFilter, includeWorktrees)
	if err != nil {
		return err
	}
	if len(currentRepos) == 0 {
		return fmt.Errorf("current context matched no repos")
	}

	removalRepos, err := resolveCommandRepos(m, wsHome, removal, includeWorktrees)
	if err != nil {
		return err
	}
	if len(removalRepos) == 0 && removal != "all" {
		return fmt.Errorf("filter %q matched no repos", removal)
	}

	remaining := subtractContextRepos(currentRepos, removalRepos)
	if len(remaining) == 0 {
		return fmt.Errorf("remove would leave the context empty")
	}

	// `remove` persists the final effective scope as explicit targets.
	// Do not re-expand worktrees when writing it back, or a removed
	// `repo@worktree` target would immediately be added again.
	return SetContext(m, wsHome, repoFilter(remaining), false)
}

// GetContext reads the current context filter, or "" if none is set.
func GetContext(wsHome string) string {
	return storedContextRaw(wsHome)
}

// GetDefaultContext returns the resolved default filter when context has been set.
// The returned bool reports whether a generated default exists.
func GetDefaultContext(wsHome string) (string, bool) {
	return getDefaultContextState(nil, wsHome, true)
}

// GetDefaultContextForMode returns the resolved default filter for the requested
// worktree mode. When worktrees are disabled, saved explicit worktree targets
// collapse back to their primary repo names.
func GetDefaultContextForMode(m *manifest.Manifest, wsHome string, includeWorktrees bool) (string, bool) {
	return getDefaultContextState(m, wsHome, includeWorktrees)
}

func getDefaultContextState(m *manifest.Manifest, wsHome string, includeWorktrees bool) (string, bool) {
	state, ok, err := loadStoredContextState(wsHome)
	if err != nil || !ok {
		return "", false
	}
	resolved := state.Resolved
	if !includeWorktrees {
		resolved = collapseResolvedContextNames(m, resolved)
	}
	if len(resolved) == 0 {
		return manifest.EmptyFilter, true
	}
	return strings.Join(resolved, ","), true
}

func collapseResolvedContextNames(m *manifest.Manifest, names []string) []string {
	if len(names) == 0 {
		return nil
	}
	if m == nil {
		return append([]string(nil), names...)
	}

	active := m.ActiveRepos()
	seen := make(map[string]bool, len(names))
	collapsed := make([]string, 0, len(names))
	for _, name := range names {
		if repoName, selector, ok := splitWorktreeToken(name, active); ok && selector != "" {
			name = repoName
		}
		if seen[name] {
			continue
		}
		seen[name] = true
		collapsed = append(collapsed, name)
	}
	return collapsed
}

// ShowContext displays the current context.
func ShowContext(m *manifest.Manifest, wsHome string) {
	workspace := activeWorkspaceName()
	if workspace != "" {
		fmt.Printf("Workspace: %s\n", workspace)
	}

	state, ok, err := loadStoredContextState(wsHome)
	if err != nil || !ok {
		fmt.Println("No context set (using all repos)")
		return
	}

	if strings.TrimSpace(state.Raw) == "" {
		if len(state.Resolved) > 0 {
			fmt.Printf("No context set (%d cloned repos on disk)\n", len(state.Resolved))
			return
		}
		fmt.Println("No context set (using all repos)")
		return
	}

	normalized, err := normalizeContextFilter(m, state.Raw)
	if err != nil {
		fmt.Printf("Context: %s (invalid: %v)\n", state.Raw, err)
		return
	}
	fmt.Printf("Context: %s (%d repos)\n", normalized, len(state.Resolved))
	if len(state.Resolved) > 0 {
		fmt.Printf("Resolved: %s\n", strings.Join(state.Resolved, ", "))
	}
}

func formatResolvedContextRepos(repos []manifest.RepoInfo) string {
	if len(repos) == 0 {
		return "(none)"
	}

	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	return strings.Join(names, ", ")
}

type scopeHint struct {
	Workspace string          `yaml:"workspace"`
	Filter    string          `yaml:"filter"`
	Repos     []scopeHintRepo `yaml:"repos"`
}

type scopeHintRepo struct {
	Name string `yaml:"name"`
	Path string `yaml:"path"`
}

func syncScopeHint(wsHome, workspace, filter string, repos []manifest.RepoInfo) error {
	hint := scopeHint{
		Workspace: workspaceLabel(workspace),
		Filter:    filter,
		Repos:     make([]scopeHintRepo, 0, len(repos)),
	}

	for _, repo := range repos {
		if !git.IsCheckout(repo.Path) {
			continue
		}
		hint.Repos = append(hint.Repos, scopeHintRepo{
			Name: repo.Name,
			Path: relWorkspacePath(wsHome, repo.Path),
		})
	}
	sort.Slice(hint.Repos, func(i, j int) bool {
		return hint.Repos[i].Name < hint.Repos[j].Name
	})

	data, err := yaml.Marshal(&hint)
	if err != nil {
		return err
	}

	path, err := stateWritePath(wsHome, workspaceScopeHintStateFile(workspace), "")
	if err != nil {
		return err
	}

	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}

	fmt.Printf("Wrote %s (%d repos)\n", filepath.ToSlash(filepath.Join(wsStateDir, workspaceScopeHintStateFile(workspace))), len(hint.Repos))
	return nil
}

func clearLegacyScopeDirs(m *manifest.Manifest, wsHome string) error {
	dirs := map[string]bool{
		manifest.DefaultScopeDir: true,
	}
	for _, cfg := range m.Scopes {
		dirs[cfg.Dir] = true
	}

	for dir := range dirs {
		if err := clearLegacyScopeDir(wsHome, dir); err != nil {
			return err
		}
	}
	return nil
}

func clearLegacyScopeDir(wsHome, dir string) error {
	absDir := filepath.Join(wsHome, dir)
	entries, err := os.ReadDir(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading legacy scope dir %s: %w", dir, err)
	}

	for _, e := range entries {
		p := filepath.Join(absDir, e.Name())
		if e.Type()&os.ModeSymlink != 0 {
			if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("clearing legacy scope dir %s: %w", dir, err)
			}
		}
	}

	return nil
}

func persistContextState(wsHome, workspace, raw string, repos []manifest.RepoInfo, previous *string) error {
	if err := saveWorkspaceContextState(wsHome, workspace, raw, repos, previous); err != nil {
		return err
	}
	if workspace == "" {
		removeLegacyResolvedContext(wsHome)
	}
	return nil
}

func normalizeContextFilter(m *manifest.Manifest, filter string) (string, error) {
	filter = strings.TrimSpace(filter)
	if filter == "" {
		return "", nil
	}

	active := m.ActiveRepos()
	tokens := strings.Split(filter, ",")
	seen := make(map[string]bool)
	var normalized []string
	var invalid []string
	hasAll := false

	for _, token := range tokens {
		token = strings.TrimSpace(token)
		if token == "" {
			continue
		}

		switch token {
		case "none", "reset":
			if len(tokens) > 1 {
				return "", fmt.Errorf("%q cannot be combined with other filters", token)
			}
			return "", nil
		case "all":
			hasAll = true
			continue
		default:
			if _, ok, err := parseActivityFilterToken(token); ok {
				if err != nil {
					invalid = append(invalid, token)
					continue
				}
				break
			}
			if _, ok := m.Groups[token]; !ok {
				if _, ok := active[token]; !ok {
					// @branch — matches all worktrees on a branch
					if strings.HasPrefix(token, "@") && len(token) > 1 {
						break
					}
					repoName, selector, worktreeTarget := splitWorktreeToken(token, active)
					if !worktreeTarget || repoName == "" || selector == "" {
						invalid = append(invalid, token)
						continue
					}
				}
			}
		}

		if !seen[token] {
			seen[token] = true
			normalized = append(normalized, token)
		}
	}

	if len(invalid) > 0 {
		return "", fmt.Errorf("unknown context filter(s): %s", strings.Join(invalid, ", "))
	}
	if hasAll {
		return "all", nil
	}
	return strings.Join(normalized, ","), nil
}

func mergeContextFilters(current, addition string) string {
	if current == "" {
		return addition
	}
	if current == "all" || addition == "all" {
		return "all"
	}

	seen := make(map[string]bool)
	var merged []string
	for _, filter := range []string{current, addition} {
		for _, token := range strings.Split(filter, ",") {
			token = strings.TrimSpace(token)
			if token == "" || seen[token] {
				continue
			}
			seen[token] = true
			merged = append(merged, token)
		}
	}
	return strings.Join(merged, ",")
}

func subtractContextRepos(current, removal []manifest.RepoInfo) []manifest.RepoInfo {
	if len(current) == 0 || len(removal) == 0 {
		return current
	}

	removeSet := make(map[string]bool, len(removal))
	for _, repo := range removal {
		removeSet[repo.Name] = true
	}

	remaining := make([]manifest.RepoInfo, 0, len(current))
	for _, repo := range current {
		if !removeSet[repo.Name] {
			remaining = append(remaining, repo)
		}
	}
	return remaining
}

func repoFilter(repos []manifest.RepoInfo) string {
	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	return strings.Join(names, ",")
}

func clonedRepos(repos []manifest.RepoInfo) []manifest.RepoInfo {
	filtered := make([]manifest.RepoInfo, 0, len(repos))
	for _, repo := range repos {
		if git.IsCheckout(repo.Path) {
			filtered = append(filtered, repo)
		}
	}
	return filtered
}
