package command

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/dtuit/ws/internal/git"
	"github.com/dtuit/ws/internal/manifest"
)

// Code generates a VS Code workspace file and opens it.
func Code(m *manifest.Manifest, wsHome, filter string) error {
	repos := m.ResolveFilter(filter, wsHome)

	wsFile := filepath.Join(wsHome, m.Workspace)

	ws := buildWorkspace(repos, wsHome)

	out, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write
	tmp := wsFile + ".tmp"
	if err := os.WriteFile(tmp, append(out, '\n'), 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, wsFile); err != nil {
		return err
	}

	fmt.Printf("Generated %s (%d repos)\n", m.Workspace, len(repos))

	// Open in VS Code if available
	if codeBin, err := exec.LookPath("code"); err == nil {
		cmd := exec.Command(codeBin, wsFile)
		cmd.Start()
	}

	return nil
}

// buildWorkspace creates the VS Code workspace JSON structure.
func buildWorkspace(repos []manifest.RepoInfo, wsHome string) map[string]interface{} {
	folders := []interface{}{
		map[string]interface{}{"name": "~ workspace", "path": "."},
	}
	seenPaths := map[string]bool{
		filepath.Clean(wsHome): true,
	}
	seenNames := map[string]int{
		"~ workspace": 1,
	}
	for _, repo := range repos {
		for _, folder := range repoFolders(repo, wsHome) {
			path := filepath.Clean(folder.Path)
			if seenPaths[path] {
				continue
			}
			relPath := relWorkspacePath(wsHome, path)
			name := uniqueFolderName(folder.Name, relPath, seenNames)
			folders = append(folders, map[string]interface{}{
				"name": name,
				"path": relPath,
			})
			seenPaths[path] = true
		}
	}

	return map[string]interface{}{
		"folders": folders,
		"settings": map[string]interface{}{
			"files.exclude": map[string]interface{}{
				"**/.git": true,
			},
		},
	}
}

type workspaceFolder struct {
	Name string
	Path string
}

func repoFolders(repo manifest.RepoInfo, wsHome string) []workspaceFolder {
	paths := []string{repo.Path}
	if worktrees, err := git.WorktreePaths(repo.Path); err == nil {
		paths = orderedWorktreePaths(repo.Path, worktrees)
	}

	folders := make([]workspaceFolder, 0, len(paths))
	for _, path := range paths {
		name := repo.Name
		if filepath.Clean(path) != filepath.Clean(repo.Path) {
			suffix := filepath.Base(path)
			if suffix == "" || suffix == "." || suffix == repo.Name {
				suffix = relWorkspacePath(wsHome, path)
			}
			name = fmt.Sprintf("%s [%s]", repo.Name, suffix)
		}
		folders = append(folders, workspaceFolder{
			Name: name,
			Path: path,
		})
	}
	return folders
}

func orderedWorktreePaths(primary string, paths []string) []string {
	primary = filepath.Clean(primary)

	ordered := []string{primary}
	seen := map[string]bool{
		primary: true,
	}
	for _, path := range paths {
		path = filepath.Clean(path)
		if seen[path] {
			continue
		}
		seen[path] = true
		ordered = append(ordered, path)
	}
	return ordered
}

func relWorkspacePath(wsHome, path string) string {
	relPath, err := filepath.Rel(wsHome, path)
	if err != nil {
		return path
	}
	return relPath
}

func uniqueFolderName(baseName, relPath string, seen map[string]int) string {
	if seen[baseName] == 0 {
		seen[baseName] = 1
		return baseName
	}

	name := fmt.Sprintf("%s (%s)", baseName, relPath)
	if seen[name] == 0 {
		seen[name] = 1
		return name
	}

	for i := 2; ; i++ {
		candidate := fmt.Sprintf("%s (%s #%d)", baseName, relPath, i)
		if seen[candidate] == 0 {
			seen[candidate] = 1
			return candidate
		}
	}
}

// BuildWorkspaceJSON is exported for testing.
func BuildWorkspaceJSON(repos []manifest.RepoInfo, wsHome string) ([]byte, error) {
	ws := buildWorkspace(repos, wsHome)
	out, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
