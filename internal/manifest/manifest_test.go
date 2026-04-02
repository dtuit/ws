package manifest

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testWSHome = "/tmp/test-ws"

const testManifest = `
remotes:
  default: git@github.com:acme-corp
  upstream: git@github.com:open-source-org

branch: main

groups:
  backend:  [api-server, auth-service, worker]
  frontend: [web-app, admin-dashboard]

repos:
  api-server:
  auth-service:
  worker: { branch: develop }
  web-app: { branch: develop }
  admin-dashboard:
  custom-tool: { url: git@custom:org/repo.git }
  upstream-lib: { remote: upstream, branch: stable }

exclude:
  - legacy-api
  - old-worker
`

func TestParse(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	assert.Equal(t, "git@github.com:acme-corp", m.Remotes["default"])
	assert.Equal(t, "git@github.com:open-source-org", m.Remotes["upstream"])
	assert.Equal(t, "main", m.DefaultBranch)
	assert.Len(t, m.Groups["backend"], 3)
	assert.Len(t, m.Groups["frontend"], 2)
	assert.Len(t, m.Repos, 7)
	assert.Len(t, m.Exclude, 2)
	assert.False(t, m.Worktrees)
}

func TestParse_BareRepoEntry(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	cfg := m.Repos["api-server"]
	assert.Empty(t, cfg.Branch)
	assert.Empty(t, cfg.Remote)
	assert.Empty(t, cfg.URL)
	assert.Equal(t, "main", m.ResolveBranch(cfg))
}

func TestParse_BranchOverride(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	cfg := m.Repos["worker"]
	assert.Equal(t, "develop", cfg.Branch)
	assert.Equal(t, "develop", m.ResolveBranch(cfg))
}

func TestResolveURL(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	tests := []struct {
		name     string
		expected string
	}{
		{"api-server", "git@github.com:acme-corp/api-server.git"},
		{"custom-tool", "git@custom:org/repo.git"},
		{"upstream-lib", "git@github.com:open-source-org/upstream-lib.git"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, m.ResolveURL(tt.name, m.Repos[tt.name]))
		})
	}
}

func TestResolveFilter_All(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("all", testWSHome)
	names := repoNames(repos)
	// Should contain all active repos from the merged manifest, grouped or not.
	assert.Contains(t, names, "api-server")
	assert.Contains(t, names, "auth-service")
	assert.Contains(t, names, "worker")
	assert.Contains(t, names, "web-app")
	assert.Contains(t, names, "admin-dashboard")
	assert.Contains(t, names, "custom-tool")
	assert.Contains(t, names, "upstream-lib")
}

func TestResolveFilter_GroupName(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("backend", testWSHome)
	names := repoNames(repos)
	assert.Equal(t, []string{"api-server", "auth-service", "worker"}, names)
}

func TestResolveFilter_CommaSeparated(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("backend,frontend", testWSHome)
	names := repoNames(repos)
	assert.Contains(t, names, "api-server")
	assert.Contains(t, names, "web-app")
	assert.Len(t, names, 5) // 3 backend + 2 frontend
}

func TestResolveFilter_SingleRepo(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("worker", testWSHome)
	assert.Len(t, repos, 1)
	assert.Equal(t, "worker", repos[0].Name)
}

func TestResolveFilter_Empty(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("", testWSHome)
	// Same as "all" - returns all active repos
	assert.Len(t, repos, 7)
}

func TestResolveFilter_AllIncludesMergedLocalRepos(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(`
root: repos
remotes:
  default: git@example.com
repos:
  repo-a:
  repo-b:
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(`
repos:
  local-repo:
`), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)

	assert.Equal(t, []string{"local-repo", "repo-a", "repo-b"}, repoNames(m.ResolveFilter("all", dir)))
	assert.Equal(t, []string{"local-repo", "repo-a", "repo-b"}, repoNames(m.ResolveFilter("", dir)))
}

func TestAllRepos(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.AllRepos(testWSHome)
	assert.Len(t, repos, 7) // all 7 active repos
	// Should be sorted
	assert.Equal(t, "admin-dashboard", repos[0].Name)
}

func TestRepoGroups(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	rg := m.RepoGroups()
	assert.Contains(t, rg["api-server"], "backend")
	assert.Contains(t, rg["web-app"], "frontend")
	assert.Empty(t, rg["custom-tool"])
}

func TestIsGroupOrRepo(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	assert.True(t, m.IsGroupOrRepo("backend"))
	assert.True(t, m.IsGroupOrRepo("worker"))
	assert.True(t, m.IsGroupOrRepo("backend,frontend"))
	assert.False(t, m.IsGroupOrRepo("nonexistent"))
	assert.False(t, m.IsGroupOrRepo("git"))
}

func TestMergeLocal(t *testing.T) {
	dir := t.TempDir()

	// Write main manifest
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(testManifest), 0644))

	// Write local override
	localYAML := `
worktrees: true

remotes:
  my-fork: git@github.com:darren

repos:
  legacy-api:
  my-experiment: { remote: my-fork, branch: dev }

exclude:
  - api-server

groups:
  my-group: [legacy-api, my-experiment]
  backend: [api-server]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(localYAML), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)

	// Remotes merged
	assert.Equal(t, "git@github.com:darren", m.Remotes["my-fork"])
	assert.Equal(t, "git@github.com:acme-corp", m.Remotes["default"]) // preserved

	// Repos merged: legacy-api un-excluded, my-experiment added
	assert.Contains(t, m.Repos, "legacy-api")
	assert.Contains(t, m.Repos, "my-experiment")
	assert.Equal(t, "dev", m.Repos["my-experiment"].Branch)

	// Exclude merged: api-server added to exclude list
	assert.Contains(t, m.Exclude, "api-server")
	assert.Contains(t, m.Exclude, "old-worker") // original preserved

	// Groups: backend overridden, my-group added
	assert.Equal(t, []string{"api-server"}, m.Groups["backend"])
	assert.Equal(t, []string{"legacy-api", "my-experiment"}, m.Groups["my-group"])
	assert.Equal(t, []string{"web-app", "admin-dashboard"}, m.Groups["frontend"]) // preserved
	assert.True(t, m.Worktrees)

	// legacy-api is in both repos and exclude - repos wins, it's active
	active := m.ActiveRepos()
	assert.Contains(t, active, "legacy-api")
}

func TestMergeLocal_NoLocalFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(testManifest), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)
	assert.Len(t, m.Repos, 7) // unchanged
}

func TestParse_DefaultBranch(t *testing.T) {
	yaml := `
remotes:
  default: git@example.com
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "master", m.DefaultBranch) // default when not specified
}

func TestParse_WorktreesExplicitTrue(t *testing.T) {
	yaml := `
worktrees: true
remotes:
  default: git@example.com
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.True(t, m.Worktrees)
}

func TestMergeLocal_WorktreesCanDisable(t *testing.T) {
	dir := t.TempDir()
	mainYAML := `
worktrees: true
remotes:
  default: git@example.com
repos:
  my-repo:
`
	localYAML := `
worktrees: false
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(mainYAML), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(localYAML), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)
	assert.False(t, m.Worktrees)
}

func TestParse_RejectsPathTraversalRepoName(t *testing.T) {
	tests := []string{
		"../../etc/evil",
		"../parent",
		"comma,name",
		"sub/dir",
		"back\\slash",
		"..",
		".",
		"",
	}
	for _, name := range tests {
		t.Run(name, func(t *testing.T) {
			yaml := fmt.Sprintf("remotes:\n  default: git@example.com\nrepos:\n  %q:\n", name)
			_, err := Parse([]byte(yaml))
			assert.Error(t, err, "expected error for repo name %q", name)
		})
	}
}

func TestParse_RejectsCommaGroupName(t *testing.T) {
	_, err := Parse([]byte(`
remotes:
  default: git@example.com
groups:
  comma,group: [repo-a]
repos:
  repo-a:
`))
	assert.Error(t, err)
}

func TestValidateURL(t *testing.T) {
	valid := []string{
		"git@github.com:acme/repo.git",
		"git@bitbucket.org:org/repo.git",
		"https://github.com/acme/repo.git",
		"ssh://git@github.com/acme/repo.git",
		"git://example.com/repo.git",
		"http://example.com/repo.git",
	}
	for _, url := range valid {
		assert.NoError(t, ValidateURL(url), "should allow: %s", url)
	}

	invalid := []string{
		"ext::sh -c evil",
		"file:///etc/passwd",
		"ftp://example.com/repo",
		"--upload-pack=evil",
	}
	for _, url := range invalid {
		assert.Error(t, ValidateURL(url), "should reject: %s", url)
	}
}

func TestResolvePath_Default(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	// Default root is "..", so path = wsHome/../repo-name
	path := m.ResolvePath("/home/user/workspace", "api-server", m.Repos["api-server"])
	assert.Equal(t, "/home/user/api-server", path)
}

func TestResolvePath_PerRepoRoot(t *testing.T) {
	yaml := `
remotes:
  default: git@example.com
repos:
  normal-repo:
  special-repo: { root: /opt/external }
  relative-repo: { root: vendor }
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)

	wsHome := "/home/user/workspace"

	// Normal repo uses manifest default root (..)
	assert.Equal(t, "/home/user/normal-repo", m.ResolvePath(wsHome, "normal-repo", m.Repos["normal-repo"]))

	// Absolute per-repo root
	assert.Equal(t, "/opt/external/special-repo", m.ResolvePath(wsHome, "special-repo", m.Repos["special-repo"]))

	// Relative per-repo root (resolved against wsHome)
	assert.Equal(t, "/home/user/workspace/vendor/relative-repo", m.ResolvePath(wsHome, "relative-repo", m.Repos["relative-repo"]))
}

func TestAllRepos_PathPopulated(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.AllRepos("/home/user/workspace")
	for _, r := range repos {
		assert.NotEmpty(t, r.Path, "Path should be populated for %s", r.Name)
		assert.Contains(t, r.Path, r.Name)
	}
}

func repoNames(repos []RepoInfo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names
}
