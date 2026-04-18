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

func writeWorkspace(m *manifest.Manifest, wsHome string, repos []manifest.RepoInfo, includeWorktrees bool) error {
	wsFile := filepath.Join(wsHome, m.Workspace)

	// Filter to cloned repos — avoids VS Code showing missing folders when
	// a context is set before the repos are cloned.
	cloned := make([]manifest.RepoInfo, 0, len(repos))
	for _, repo := range repos {
		if git.IsCheckout(repo.Path) {
			cloned = append(cloned, repo)
		}
	}
	worktreeCount := workspaceWorktreeCount(cloned, wsHome)

	ws := buildWorkspace(cloned, wsHome, includeWorktrees)

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

	fmt.Println(workspaceSummary(m.Workspace, len(cloned), worktreeCount, includeWorktrees))
	return nil
}

// ResolveEditor returns the editor command to use, checking (in order):
// 1. the --editor flag value (if non-empty)
// 2. the WS_EDITOR environment variable
// 3. the default "code"
func ResolveEditor(flagValue string) string {
	if flagValue != "" {
		return flagValue
	}
	if env := os.Getenv("WS_EDITOR"); env != "" {
		return env
	}
	return "code"
}

// Open opens the generated workspace file with the given editor.
func Open(m *manifest.Manifest, wsHome string, editor string) error {
	wsFile := filepath.Join(wsHome, m.Workspace)
	if _, err := os.Stat(wsFile); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("%s not found; run `ws context ...` first", m.Workspace)
		}
		return err
	}

	bin, err := exec.LookPath(editor)
	if err != nil {
		return fmt.Errorf("`%s` command not found in PATH", editor)
	}

	cmd := exec.Command(bin, wsFile)
	if err := cmd.Start(); err != nil {
		return err
	}
	fmt.Printf("Opened %s with %s\n", m.Workspace, editor)
	return nil
}

// buildWorkspace creates the VS Code workspace JSON structure.
func buildWorkspace(repos []manifest.RepoInfo, wsHome string, includeWorktrees bool) map[string]interface{} {
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
		for _, folder := range repoFolders(repo, wsHome, includeWorktrees) {
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

func repoFolders(repo manifest.RepoInfo, wsHome string, includeWorktrees bool) []workspaceFolder {
	paths := []string{repo.Path}
	if includeWorktrees && repo.Worktree == "" {
		worktrees, err := git.WorktreePaths(repo.Path)
		if err == nil {
			paths = orderedWorktreePaths(repo.Path, worktrees)
		}
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

func workspaceWorktreeCount(repos []manifest.RepoInfo, wsHome string) int {
	count := 0
	for _, repo := range repos {
		count += len(repoFolders(repo, wsHome, true)) - 1
	}
	return count
}

func workspaceSummary(workspace string, repoCount, worktreeCount int, includeWorktrees bool) string {
	repoLabel := "repos"
	if repoCount == 1 {
		repoLabel = "repo"
	}

	if includeWorktrees {
		if worktreeCount == 1 {
			return fmt.Sprintf("Generated VS Code workspace %s (%d %s, 1 worktree)", workspace, repoCount, repoLabel)
		}
		return fmt.Sprintf("Generated VS Code workspace %s (%d %s, %d worktrees)", workspace, repoCount, repoLabel, worktreeCount)
	}

	return fmt.Sprintf("Generated VS Code workspace %s (%d %s, worktrees disabled)", workspace, repoCount, repoLabel)
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
func BuildWorkspaceJSON(repos []manifest.RepoInfo, wsHome string, includeWorktrees bool) ([]byte, error) {
	ws := buildWorkspace(repos, wsHome, includeWorktrees)
	out, err := json.MarshalIndent(ws, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
