package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

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

func commitEmptyAt(t *testing.T, repoDir, message, name, email string, when time.Time) {
	t.Helper()
	cmd := exec.Command("git", "commit", "--allow-empty", "-m", message)
	cmd.Dir = repoDir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME="+name,
		"GIT_AUTHOR_EMAIL="+email,
		"GIT_COMMITTER_NAME="+name,
		"GIT_COMMITTER_EMAIL="+email,
		"GIT_AUTHOR_DATE="+when.UTC().Format(time.RFC3339),
		"GIT_COMMITTER_DATE="+when.UTC().Format(time.RFC3339),
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git commit failed: %s", out)
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

func TestInspectRepoActivity_DirtyRepoMatches(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "dirty-repo")
	cmd := exec.Command("git", "config", "user.email", "other@example.com")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.name", "Other User")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "dirty.txt"), []byte("dirty"), 0644))

	activity := InspectRepoActivity(manifest.RepoInfo{Name: "dirty-repo", Path: repoDir, Branch: "master"}, 0)
	assert.NoError(t, activity.Err)
	assert.True(t, activity.Dirty)
	assert.False(t, activity.RecentLocalCommit)
}

func TestInspectRepoActivity_RecentLocalCommitMatchesWithinWindow(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "recent-repo")
	cmd := exec.Command("git", "config", "user.email", "local@example.com")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.name", "Local User")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	commitEmptyAt(t, repoDir, "recent work", "Local User", "local@example.com", time.Now().Add(-48*time.Hour))

	activity := InspectRepoActivity(manifest.RepoInfo{Name: "recent-repo", Path: repoDir, Branch: "master"}, 7*24*time.Hour)
	assert.NoError(t, activity.Err)
	assert.False(t, activity.Dirty)
	assert.True(t, activity.RecentLocalCommit)
}

func TestInspectRepoActivity_RecentLocalCommitRespectsWindow(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "old-repo")
	cmd := exec.Command("git", "config", "user.email", "old@example.com")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.name", "Old User")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	commitEmptyAt(t, repoDir, "recent enough for a week, not for a day", "Old User", "old@example.com", time.Now().Add(-48*time.Hour))

	activity := InspectRepoActivity(manifest.RepoInfo{Name: "old-repo", Path: repoDir, Branch: "master"}, 24*time.Hour)
	assert.NoError(t, activity.Err)
	assert.False(t, activity.Dirty)
	assert.False(t, activity.RecentLocalCommit)
}

func TestInspectRepoActivity_LinkedWorktreeDirtyMatches(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "worktree-repo")
	cmd := exec.Command("git", "config", "user.email", "other@example.com")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "config", "user.name", "Other User")
	cmd.Dir = repoDir
	require.NoError(t, cmd.Run())
	worktreeDir := addTestWorktree(t, repoDir, "worktree-feature", "feature/auto")
	require.NoError(t, os.WriteFile(filepath.Join(worktreeDir, "dirty.txt"), []byte("dirty"), 0644))

	activity := InspectRepoActivity(manifest.RepoInfo{Name: "worktree-repo", Path: repoDir, Branch: "master"}, 0)
	assert.NoError(t, activity.Err)
	assert.True(t, activity.Dirty)
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

func TestCompareSymbol_Empty(t *testing.T) {
	s := RepoStatus{}
	assert.Equal(t, "", s.CompareSymbol())
}

func TestCompareSymbol_NoRef(t *testing.T) {
	s := RepoStatus{CompareRemote: "upstream", CompareNoRef: true}
	assert.Equal(t, "~", s.CompareSymbol())
}

func TestCompareSymbol_AheadBehindDiverged(t *testing.T) {
	cases := []struct {
		s    RepoStatus
		want string
	}{
		{RepoStatus{CompareRemote: "upstream"}, "="},
		{RepoStatus{CompareRemote: "upstream", CompareAhead: 3}, "↑3"},
		{RepoStatus{CompareRemote: "upstream", CompareBehind: 2}, "↓2"},
		{RepoStatus{CompareRemote: "upstream", CompareAhead: 1, CompareBehind: 2}, "1⇕2"},
	}
	for _, tc := range cases {
		assert.Equal(t, tc.want, tc.s.CompareSymbol())
	}
}

func TestStatusAll_PopulatesCompare(t *testing.T) {
	dir := t.TempDir()

	upstream := initTestRepo(t, dir, "upstream-src")

	// Clone fork from upstream so they share history (the realistic setup).
	fork := filepath.Join(dir, "fork")
	cmd := exec.Command("git", "clone", upstream, fork)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "clone failed: %s", out)

	// git clone names the source remote "origin"; rename it to "upstream"
	// so the comparison resolves against upstream/master.
	for _, args := range [][]string{
		{"git", "remote", "rename", "origin", "upstream"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
	} {
		cmd := exec.Command(args[0], args[1:]...)
		cmd.Dir = fork
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}

	// Diverge: 2 new commits on fork, 1 new commit on upstream.
	commitEmptyAt(t, fork, "fork-1", "Test", "test@test.com", time.Now())
	commitEmptyAt(t, fork, "fork-2", "Test", "test@test.com", time.Now())
	commitEmptyAt(t, upstream, "upstream-1", "Test", "test@test.com", time.Now())
	cmd = exec.Command("git", "fetch", "upstream")
	cmd.Dir = fork
	out, err = cmd.CombinedOutput()
	require.NoError(t, err, "fetch failed: %s", out)

	results := StatusAll([]manifest.RepoInfo{
		{Name: "fork", Path: fork, DefaultCompare: "upstream"},
	}, 1)
	require.Len(t, results, 1)
	s := results[0]
	require.NoError(t, s.Err)
	assert.Equal(t, "upstream", s.CompareRemote)
	assert.False(t, s.CompareNoRef)
	assert.Equal(t, 2, s.CompareAhead)
	assert.Equal(t, 1, s.CompareBehind)
	assert.Equal(t, "2⇕1", s.CompareSymbol())
}

func TestStatusAll_CompareNoRefWhenRemoteMissing(t *testing.T) {
	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "lonely")

	results := StatusAll([]manifest.RepoInfo{
		{Name: "lonely", Path: repoDir, DefaultCompare: "upstream"},
	}, 1)
	require.Len(t, results, 1)
	s := results[0]
	require.NoError(t, s.Err)
	assert.Equal(t, "upstream", s.CompareRemote)
	assert.True(t, s.CompareNoRef)
	assert.Equal(t, "~", s.CompareSymbol())
}

func TestStatusAll_CompareExplicitBranch(t *testing.T) {
	dir := t.TempDir()
	upstream := initTestRepo(t, dir, "upstream-src")

	fork := filepath.Join(dir, "fork")
	cmd := exec.Command("git", "clone", upstream, fork)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "clone failed: %s", out)

	for _, args := range [][]string{
		{"git", "remote", "rename", "origin", "upstream"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		// Local fork is on a divergent branch name (mirrors xtracta/bifrost).
		{"git", "checkout", "-b", "xtracta-main"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = fork
		out, err := c.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}

	commitEmptyAt(t, fork, "fork-1", "Test", "test@test.com", time.Now())
	commitEmptyAt(t, upstream, "upstream-1", "Test", "test@test.com", time.Now())
	c := exec.Command("git", "fetch", "upstream")
	c.Dir = fork
	out, err = c.CombinedOutput()
	require.NoError(t, err, "fetch failed: %s", out)

	// Explicit form: pin the compare branch to upstream's master regardless of
	// the local branch name.
	results := StatusAll([]manifest.RepoInfo{
		{Name: "fork", Path: fork, DefaultCompare: "upstream:master"},
	}, 1)
	require.Len(t, results, 1)
	s := results[0]
	require.NoError(t, s.Err)
	assert.Equal(t, "upstream", s.CompareRemote)
	assert.False(t, s.CompareNoRef)
	assert.Equal(t, 1, s.CompareAhead)
	assert.Equal(t, 1, s.CompareBehind)
}

func TestStatusAll_CompareFallsBackToRemoteHEAD(t *testing.T) {
	dir := t.TempDir()
	upstream := initTestRepo(t, dir, "upstream-src")

	fork := filepath.Join(dir, "fork")
	cmd := exec.Command("git", "clone", upstream, fork)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "clone failed: %s", out)

	for _, args := range [][]string{
		{"git", "remote", "rename", "origin", "upstream"},
		{"git", "config", "user.email", "test@test.com"},
		{"git", "config", "user.name", "Test"},
		{"git", "checkout", "-b", "xtracta-main"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = fork
		out, err := c.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}

	commitEmptyAt(t, fork, "fork-1", "Test", "test@test.com", time.Now())
	commitEmptyAt(t, upstream, "upstream-1", "Test", "test@test.com", time.Now())

	// Set remote HEAD so the fallback has somewhere to resolve to.
	for _, args := range [][]string{
		{"git", "fetch", "upstream"},
		{"git", "remote", "set-head", "upstream", "master"},
	} {
		c := exec.Command(args[0], args[1:]...)
		c.Dir = fork
		out, err := c.CombinedOutput()
		require.NoError(t, err, "cmd %v failed: %s", args, out)
	}

	// Bare form: remote/<local-branch> doesn't exist (xtracta-main isn't on
	// upstream), so populateCompare should fall back to upstream/HEAD.
	results := StatusAll([]manifest.RepoInfo{
		{Name: "fork", Path: fork, DefaultCompare: "upstream"},
	}, 1)
	require.Len(t, results, 1)
	s := results[0]
	require.NoError(t, s.Err)
	assert.Equal(t, "upstream", s.CompareRemote)
	assert.False(t, s.CompareNoRef)
	assert.Equal(t, 1, s.CompareAhead)
	assert.Equal(t, 1, s.CompareBehind)
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

func TestParseLocalBranchTrack(t *testing.T) {
	ahead, behind, noRemote := parseLocalBranchTrack("origin/main", "[ahead 2, behind 1]")
	assert.Equal(t, 2, ahead)
	assert.Equal(t, 1, behind)
	assert.False(t, noRemote)

	ahead, behind, noRemote = parseLocalBranchTrack("origin/main", "")
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
	assert.False(t, noRemote)

	ahead, behind, noRemote = parseLocalBranchTrack("", "")
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
	assert.True(t, noRemote)

	ahead, behind, noRemote = parseLocalBranchTrack("origin/main", "[gone]")
	assert.Equal(t, 0, ahead)
	assert.Equal(t, 0, behind)
	assert.True(t, noRemote)
}

func TestParseLocalBranchList(t *testing.T) {
	out := "*\tmaster\torigin/master\t[ahead 1]\tinitial commit\t2 days ago\n \tfeature/auth\t\t\tadd auth\t1 hour ago\n"

	branches, err := parseLocalBranchList(out)
	require.NoError(t, err)
	require.Len(t, branches, 2)

	assert.Equal(t, "master", branches[0].Name)
	assert.True(t, branches[0].Current)
	assert.Equal(t, 1, branches[0].Ahead)
	assert.Equal(t, 0, branches[0].Behind)
	assert.False(t, branches[0].NoRemote)
	assert.Equal(t, "initial commit", branches[0].CommitMsg)
	assert.Equal(t, "2 days ago", branches[0].CommitAge)

	assert.Equal(t, "feature/auth", branches[1].Name)
	assert.False(t, branches[1].Current)
	assert.True(t, branches[1].NoRemote)
	assert.Equal(t, "add auth", branches[1].CommitMsg)
	assert.Equal(t, "1 hour ago", branches[1].CommitAge)
}
