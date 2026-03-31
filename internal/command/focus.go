package command

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/dtuit/ws/internal/manifest"
)

// Focus updates the VS Code workspace file to show only the filtered repos.
func Focus(m *manifest.Manifest, parentDir, wsHome, filter string) error {
	repos := m.ResolveFilter(filter)

	wsFile, err := findWorkspaceFile(wsHome)
	if err != nil {
		return err
	}

	data, err := os.ReadFile(wsFile)
	if err != nil {
		return fmt.Errorf("reading workspace file: %w", err)
	}

	var ws map[string]interface{}
	if err := json.Unmarshal(data, &ws); err != nil {
		return fmt.Errorf("parsing workspace file: %w", err)
	}

	// Keep first folder ("~ workspace"), replace the rest
	folders, ok := ws["folders"].([]interface{})
	if !ok || len(folders) == 0 {
		return fmt.Errorf("workspace file has no folders")
	}

	// Compute relative path prefix from workspace dir to repo root
	relRoot, err := filepath.Rel(wsHome, parentDir)
	if err != nil {
		relRoot = parentDir // fallback to absolute
	}

	newFolders := []interface{}{folders[0]}
	for _, repo := range repos {
		newFolders = append(newFolders, map[string]interface{}{
			"name": repo.Name,
			"path": filepath.Join(relRoot, repo.Name),
		})
	}
	ws["folders"] = newFolders

	out, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}
	// Atomic write: temp file + rename
	tmp := wsFile + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, wsFile); err != nil {
		return err
	}

	allGrouped := m.ResolveFilter("all")
	if filter == "" || filter == "all" {
		fmt.Printf("Focus: showing all %d repos\n", len(repos))
	} else {
		fmt.Printf("Focus: showing %d repo(s), hiding %d\n", len(repos), len(allGrouped)-len(repos))
	}
	return nil
}

// findWorkspaceFile finds the .code-workspace file in the given directory.
func findWorkspaceFile(dir string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".code-workspace") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("no .code-workspace file found in %s", dir)
}

// FocusJSON is exported for testing - applies focus transformation to workspace JSON.
func FocusJSON(data []byte, repos []manifest.RepoInfo, relRoot string) ([]byte, error) {
	var ws map[string]interface{}
	if err := json.Unmarshal(data, &ws); err != nil {
		return nil, err
	}

	folders, ok := ws["folders"].([]interface{})
	if !ok || len(folders) == 0 {
		return nil, fmt.Errorf("no folders")
	}

	newFolders := []interface{}{folders[0]}
	for _, repo := range repos {
		newFolders = append(newFolders, map[string]interface{}{
			"name": repo.Name,
			"path": filepath.Join(relRoot, repo.Name),
		})
	}
	ws["folders"] = newFolders

	out, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
