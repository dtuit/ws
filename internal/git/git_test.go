package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a git repo with one commit in a temp directory.
func initTestRepo(t *testing.T, dir, name string) string {
	t.Helper()
	repoDir := filepath.Join(dir, name)
	require.NoError(t, os.MkdirAll(repoDir, 0755))

	cmds := [][]string{
		{"git", "init"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "commit", "--allow-empty", "-m", "initial commit"},
	}
	for _, args := range cmds {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}
	return repoDir
}

func addTestWorktree(t *testing.T, repoDir, name, branch string) string {
	t.Helper()
	worktreeDir := filepath.Join(filepath.Dir(repoDir), name)
	cmd := exec.Command("git", "worktree", "add", "-b", branch, worktreeDir)
	cmd.Dir = repoDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git worktree add failed: %s", out)
	return worktreeDir
}

func TestStatus_CleanRepo(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "clean-repo")

	s := Status(repoDir, "clean-repo")
	assert.NoError(t, s.Err)
	assert.Equal(t, "master", s.Branch)
	assert.False(t, s.Staged)
	assert.False(t, s.Unstaged)
	assert.False(t, s.Untracked)
	assert.False(t, s.Stashed)
	assert.False(t, s.IsDirty())
	assert.True(t, s.NoRemote) // no upstream set
	assert.Equal(t, "initial commit", s.CommitMsg)
	assert.NotEmpty(t, s.CommitAge)
}

func TestStatus_UntrackedFile(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "untracked-repo")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "new.txt"), []byte("hello"), 0644))

	s := Status(repoDir, "untracked-repo")
	assert.NoError(t, s.Err)
	assert.True(t, s.Untracked)
	assert.False(t, s.Staged)
	assert.False(t, s.Unstaged)
	assert.True(t, s.IsDirty())
	assert.Equal(t, "?", s.Symbols())
}

func TestStatus_StagedChanges(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "staged-repo")

	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "file.txt"), []byte("hello"), 0644))
	cmd := exec.Command("git", "add", "file.txt")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	s := Status(repoDir, "staged-repo")
	assert.NoError(t, s.Err)
	assert.True(t, s.Staged)
	assert.False(t, s.Unstaged)
	assert.True(t, s.IsDirty())
	assert.Contains(t, s.Symbols(), "+")
}

func TestStatus_UnstagedChanges(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "unstaged-repo")

	// Create and commit a file, then use git to modify working tree
	fpath := filepath.Join(repoDir, "file.txt")
	require.NoError(t, os.WriteFile(fpath, []byte("v1"), 0644))
	for _, args := range [][]string{
		{"git", "add", "."},
		{"git", "commit", "-m", "add file"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v: %s", args, out)
	}

	// Use git update-index to refresh, then modify
	cmd := exec.Command("git", "status")
	cmd.Dir = repoDir
	cmd.Run() // refresh stat cache

	require.NoError(t, os.WriteFile(fpath, []byte("v2-modified-content"), 0644))

	s := Status(repoDir, "unstaged-repo")
	assert.NoError(t, s.Err)
	assert.True(t, s.Unstaged, "expected unstaged changes")
	assert.False(t, s.Staged, "expected no staged changes")
	assert.Contains(t, s.Symbols(), "*")
}

func TestStatus_MixedDirtyState(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "mixed-repo")

	// Staged file
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "staged.txt"), []byte("s"), 0644))
	cmd := exec.Command("git", "add", "staged.txt")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	// Untracked file
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("u"), 0644))

	s := Status(repoDir, "mixed-repo")
	assert.True(t, s.Staged)
	assert.True(t, s.Untracked)
	assert.Contains(t, s.Symbols(), "+")
	assert.Contains(t, s.Symbols(), "?")
}

func TestStatus_DetachedHead(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "detached-repo")

	cmd := exec.Command("git", "checkout", "--detach", "HEAD")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())

	s := Status(repoDir, "detached-repo")
	assert.NoError(t, s.Err)
	assert.Equal(t, "(detached)", s.Branch)
}

func TestStatus_MissingDir(t *testing.T) {
	s := Status("/nonexistent/path", "missing")
	assert.Error(t, s.Err)
	assert.Contains(t, s.Err.Error(), "not cloned")
}

func TestStatus_NoRemote(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "no-remote")

	s := Status(repoDir, "no-remote")
	assert.True(t, s.NoRemote)
	assert.Equal(t, "~", s.SyncSymbol())
}

func TestStatus_WorktreeStashUsesCommonDir(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "stash-repo")
	trackedFile := filepath.Join(repoDir, "tracked.txt")
	require.NoError(t, os.WriteFile(trackedFile, []byte("v1"), 0644))
	for _, args := range [][]string{
		{"git", "add", "tracked.txt"},
		{"git", "commit", "-m", "add tracked file"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}
	worktreeDir := addTestWorktree(t, repoDir, "stash-feature", "feature/stash")

	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "tracked.txt"), []byte("stash me"), 0644))
	cmd := exec.Command("git", "stash", "push", "-m", "worktree stash")
	cmd.Dir = worktreeDir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git stash failed: %s", out)

	s := Status(worktreeDir, "stash-feature")
	assert.NoError(t, s.Err)
	assert.True(t, s.Stashed)
	assert.Equal(t, "feature/stash", s.Branch)
}

func TestDiscoverWorktrees_LinkedCheckout(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "repo")
	worktreeDir := addTestWorktree(t, repoDir, "repo-feature", "feature/list")

	repo := manifest.RepoInfo{Name: "repo", Path: repoDir, Branch: "master"}
	worktrees, err := DiscoverWorktrees(repo)
	require.NoError(t, err)
	require.Len(t, worktrees, 2)

	assert.Equal(t, filepath.Clean(repoDir), worktrees[0].Path)
	assert.True(t, worktrees[0].Primary)
	assert.Equal(t, "master", worktrees[0].Branch)
	assert.Equal(t, filepath.Join(repoDir, ".git"), worktrees[0].CommonDir)

	assert.Equal(t, filepath.Clean(worktreeDir), worktrees[1].Path)
	assert.False(t, worktrees[1].Primary)
	assert.Equal(t, "feature/list", worktrees[1].Branch)
	assert.Equal(t, filepath.Join(repoDir, ".git"), worktrees[1].CommonDir)
}

func TestDiscoverWorktrees_PrimaryTracksManifestPath(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "repo")
	worktreeDir := addTestWorktree(t, repoDir, "repo-feature", "feature/primary")

	repo := manifest.RepoInfo{Name: "repo", Path: worktreeDir, Branch: "feature/primary"}
	worktrees, err := DiscoverWorktrees(repo)
	require.NoError(t, err)
	require.Len(t, worktrees, 2)

	assert.Equal(t, filepath.Clean(worktreeDir), worktrees[0].Path)
	assert.True(t, worktrees[0].Primary)
	assert.Equal(t, "feature/primary", worktrees[0].Branch)

	assert.Equal(t, filepath.Clean(repoDir), worktrees[1].Path)
	assert.False(t, worktrees[1].Primary)
}

func TestSymbols_Empty(t *testing.T) {
	s := RepoStatus{}
	assert.Equal(t, "", s.Symbols())
}

func TestSyncSymbol_InSync(t *testing.T) {
	s := RepoStatus{Ahead: 0, Behind: 0, NoRemote: false}
	assert.Equal(t, "=", s.SyncSymbol())
}

func TestSyncSymbol_Ahead(t *testing.T) {
	s := RepoStatus{Ahead: 3}
	assert.Equal(t, "↑3", s.SyncSymbol())
}

func TestSyncSymbol_Behind(t *testing.T) {
	s := RepoStatus{Behind: 2}
	assert.Equal(t, "↓2", s.SyncSymbol())
}

func TestSyncSymbol_Diverged(t *testing.T) {
	s := RepoStatus{Ahead: 1, Behind: 2}
	assert.Equal(t, "1⇕2", s.SyncSymbol())
}

func TestStatusAll_Parallel(t *testing.T) {
	dir := t.TempDir()

	repos := []manifest.RepoInfo{
		{Name: "repo-a", Path: filepath.Join(dir, "repo-a")},
		{Name: "repo-b", Path: filepath.Join(dir, "repo-b")},
		{Name: "repo-c", Path: filepath.Join(dir, "repo-c")},
	}
	for _, r := range repos {
		initTestRepo(t, dir, r.Name)
	}

	results := StatusAll(repos, 5)
	assert.Len(t, results, 3)
	for i, s := range results {
		assert.NoError(t, s.Err, "repo %s", repos[i].Name)
		assert.Equal(t, "master", s.Branch)
	}
}

func TestStatusAll_MixedState(t *testing.T) {
	dir := t.TempDir()

	initTestRepo(t, dir, "exists")
	repos := []manifest.RepoInfo{
		{Name: "exists", Path: filepath.Join(dir, "exists")},
		{Name: "missing", Path: filepath.Join(dir, "missing")},
	}

	results := StatusAll(repos, 5)
	assert.NoError(t, results[0].Err)
	assert.Error(t, results[1].Err)
}

func TestWorkers_Default(t *testing.T) {
	os.Unsetenv("WS_WORKERS")
	w := Workers(100)
	assert.Greater(t, w, 0)
	// Should be capped at CPU count
	w2 := Workers(1)
	assert.Equal(t, 1, w2) // min(cpus, 1) = 1
}

func TestWorkers_EnvOverride(t *testing.T) {
	t.Setenv("WS_WORKERS", "8")
	assert.Equal(t, 8, Workers(100))
}
