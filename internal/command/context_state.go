package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
	"gopkg.in/yaml.v3"
)

const (
	// Legacy flat paths kept for read-only fallback; new state lives under .ws/.
	legacyContextFile         = ".ws-context"
	legacyResolvedContextFile = ".ws-context.resolved"

	contextStateFile   = "context.yml"
	scopeHintFile      = "scope.yml"
	workspacesStateDir = "workspaces"
)

type contextState struct {
	Raw      string   `yaml:"raw"`
	Resolved []string `yaml:"resolved"`
	// Previous holds the prior raw filter for `ws context -` swap.
	// A nil pointer means "no previous recorded"; a pointer to "" means
	// the prior context was explicitly cleared (swap-to-cleared is valid).
	Previous *string `yaml:"previous,omitempty"`
}

func activeWorkspaceName() string {
	return strings.TrimSpace(os.Getenv("WS_WORKSPACE"))
}

func workspaceContextStateFile(workspace string) string {
	if workspace == "" {
		return contextStateFile
	}
	return filepath.Join(workspacesStateDir, workspace, contextStateFile)
}

func workspaceScopeHintStateFile(workspace string) string {
	if workspace == "" {
		return scopeHintFile
	}
	return filepath.Join(workspacesStateDir, workspace, scopeHintFile)
}

func loadStoredContextState(wsHome string) (contextState, bool, error) {
	return loadWorkspaceContextState(wsHome, activeWorkspaceName())
}

func saveStoredContextState(wsHome, raw string, repos []manifest.RepoInfo, previous *string) error {
	return saveWorkspaceContextState(wsHome, activeWorkspaceName(), raw, repos, previous)
}

func loadWorkspaceContextState(wsHome, workspace string) (contextState, bool, error) {
	legacy := ""
	if workspace == "" {
		legacy = legacyContextFile
	}
	return readContextState(stateReadPath(wsHome, workspaceContextStateFile(workspace), legacy))
}

func saveWorkspaceContextState(wsHome, workspace, raw string, repos []manifest.RepoInfo, previous *string) error {
	legacy := ""
	if workspace == "" {
		legacy = legacyContextFile
	}
	path, err := stateWritePath(wsHome, workspaceContextStateFile(workspace), legacy)
	if err != nil {
		return err
	}
	return writeContextState(path, raw, repos, previous)
}

func readContextState(path string) (contextState, bool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return contextState{}, false, nil
		}
		return contextState{}, false, err
	}

	var state contextState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return contextState{}, false, err
	}
	return state, true, nil
}

func writeContextState(path, raw string, repos []manifest.RepoInfo, previous *string) error {
	state := contextState{
		Raw:      raw,
		Resolved: resolvedContextNames(repos),
		Previous: previous,
	}
	data, err := yaml.Marshal(&state)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func resolvedContextNames(repos []manifest.RepoInfo) []string {
	if len(repos) == 0 {
		return nil
	}

	names := make([]string, 0, len(repos))
	for _, repo := range repos {
		names = append(names, repo.Name)
	}
	return names
}

func removeLegacyResolvedContext(wsHome string) {
	path := filepath.Join(wsHome, legacyResolvedContextFile)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "Warning: could not remove %s: %v\n", legacyResolvedContextFile, err)
	}
}

func storedContextRaw(wsHome string) string {
	state, ok, err := loadStoredContextState(wsHome)
	if err != nil || !ok {
		return ""
	}
	return strings.TrimSpace(state.Raw)
}
