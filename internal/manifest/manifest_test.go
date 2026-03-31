package manifest

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestParse_BackwardCompat_SingularRemote(t *testing.T) {
	yaml := `
remote: git@github.com:acme-corp
branch: main
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)

	assert.Equal(t, "git@github.com:acme-corp", m.Remotes["default"])
	assert.Equal(t, "git@github.com:acme-corp/my-repo.git", m.ResolveURL("my-repo", m.Repos["my-repo"]))
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

	repos := m.ResolveFilter("all")
	names := repoNames(repos)
	// Should contain all repos that are in at least one group
	assert.Contains(t, names, "api-server")
	assert.Contains(t, names, "auth-service")
	assert.Contains(t, names, "worker")
	assert.Contains(t, names, "web-app")
	assert.Contains(t, names, "admin-dashboard")
	// Should NOT contain repos not in any group
	assert.NotContains(t, names, "custom-tool")
	assert.NotContains(t, names, "upstream-lib")
}

func TestResolveFilter_GroupName(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("backend")
	names := repoNames(repos)
	assert.Equal(t, []string{"api-server", "auth-service", "worker"}, names)
}

func TestResolveFilter_CommaSeparated(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("backend,frontend")
	names := repoNames(repos)
	assert.Contains(t, names, "api-server")
	assert.Contains(t, names, "web-app")
	assert.Len(t, names, 5) // 3 backend + 2 frontend
}

func TestResolveFilter_SingleRepo(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("worker")
	assert.Len(t, repos, 1)
	assert.Equal(t, "worker", repos[0].Name)
}

func TestResolveFilter_Empty(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.ResolveFilter("")
	// Same as "all" - returns grouped repos
	assert.Len(t, repos, 5)
}

func TestAllRepos(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	repos := m.AllRepos()
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

func repoNames(repos []RepoInfo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names
}
