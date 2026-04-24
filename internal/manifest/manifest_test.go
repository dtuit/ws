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
root: ..

remotes:
  origin: git@github.com:acme-corp

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
  custom-tool:
    remotes:
      origin: git@custom:org/repo.git
  upstream-lib:
    branch: stable
    remotes:
      upstream: git@github.com:open-source-org/upstream-lib.git
    default_compare: upstream

exclude:
  - legacy-api
  - old-worker
`

func TestParse(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	assert.Equal(t, "git@github.com:acme-corp", m.Remotes["origin"])
	assert.Equal(t, "main", m.DefaultBranch)
	assert.Len(t, m.Groups["backend"], 3)
	assert.Len(t, m.Groups["frontend"], 2)
	assert.Len(t, m.Repos, 7)
	assert.Len(t, m.Exclude, 2)
	assert.False(t, m.Worktrees)
	require.Len(t, m.Scopes, 1)
	assert.Equal(t, ScopeDirConfig{Dir: DefaultScopeDir, Source: ScopeSourceContext}, m.Scopes[0])
}

func TestParse_CustomScopes(t *testing.T) {
	m, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
scopes:
  - dir: .scope
    source: context
  - dir: .all
    source: all
repos:
  repo-a:
`))
	require.NoError(t, err)
	assert.Equal(t, []ScopeDirConfig{
		{Dir: ".scope", Source: ScopeSourceContext},
		{Dir: ".all", Source: ScopeSourceAll},
	}, m.Scopes)
}

func TestParse_RejectsInvalidScopeDir(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
scopes:
  - dir: ../outside
repos:
  repo-a:
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "dir must stay within the workspace")
}

func TestParse_RejectsInvalidScopeSource(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
scopes:
  - dir: .scope
    source: weird
repos:
  repo-a:
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown source "weird"`)
}

func TestParse_BareRepoEntry(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	cfg := m.Repos["api-server"]
	assert.Empty(t, cfg.Branch)
	assert.Empty(t, cfg.Remotes)
	assert.Empty(t, cfg.DefaultCompare)
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
		// ResolveURL returns the effective origin. upstream-lib declares an
		// `upstream` remote but its origin falls back to the top-level prefix.
		{"upstream-lib", "git@github.com:acme-corp/upstream-lib.git"},
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
  origin: git@example.com:org
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
scopes:
  - dir: .scope
    source: context
  - dir: .all
    source: all
worktrees: true

repos:
  legacy-api:
  my-experiment:
    branch: dev
    remotes:
      origin: git@github.com:darren/my-experiment.git

exclude:
  - api-server

groups:
  my-group: [legacy-api, my-experiment]
  backend: [api-server]
`
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(localYAML), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)

	// Top-level origin prefix preserved from main
	assert.Equal(t, "git@github.com:acme-corp", m.Remotes["origin"])
	// my-experiment's origin comes from its per-repo literal
	assert.Equal(t, "git@github.com:darren/my-experiment.git", m.ResolveURL("my-experiment", m.Repos["my-experiment"]))
	assert.Equal(t, []ScopeDirConfig{
		{Dir: ".scope", Source: ScopeSourceContext},
		{Dir: ".all", Source: ScopeSourceAll},
	}, m.Scopes)

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

func TestMergeLocal_ScopesCanBeDisabled(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(`
root: ..
remotes:
  origin: git@example.com:org
repos:
  repo-a:
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(`
scopes: []
`), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)
	assert.Empty(t, m.Scopes)
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
root: ..
remotes:
  origin: git@example.com:org
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, "master", m.DefaultBranch) // default when not specified
}

func TestParse_WorktreesExplicitTrue(t *testing.T) {
	yaml := `
root: ..
worktrees: true
remotes:
  origin: git@example.com:org
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
root: ..
worktrees: true
remotes:
  origin: git@example.com:org
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

func TestParse_MuxBarsTrue(t *testing.T) {
	yaml := `
root: ..
mux:
  backend: tmux
  bars: true
  windows:
    - {name: editor, dir: my-repo}
remotes:
  origin: git@example.com:org
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.True(t, m.Mux.Bars)
	assert.Equal(t, "tmux", m.Mux.Backend)
	assert.Len(t, m.Mux.Windows, 1)
}

func TestParse_MuxWindowSizes(t *testing.T) {
	yaml := `
root: ..
mux:
  windows:
    - {name: dev, dir: api, panes: 2, sizes: [70, 30], layout: even-horizontal}
remotes:
  origin: git@example.com:org
repos:
  api:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	require.Len(t, m.Mux.Windows, 1)
	assert.Equal(t, []int{70, 30}, m.Mux.Windows[0].Sizes)
}

func TestParse_MuxCmdScalar(t *testing.T) {
	yaml := `
root: ..
mux:
  windows:
    - {name: dev, dir: api, cmd: cc}
remotes:
  origin: git@example.com:org
repos:
  api:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, []string{"cc"}, m.Mux.Windows[0].Cmd)
}

func TestParse_MuxCmdList(t *testing.T) {
	yaml := `
root: ..
mux:
  windows:
    - name: dev
      dir: api
      panes: 2
      cmd: [cc, ""]
remotes:
  origin: git@example.com:org
repos:
  api:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Equal(t, []string{"cc", ""}, m.Mux.Windows[0].Cmd)
}

func TestParse_MuxNamedSessions(t *testing.T) {
	yaml := `
root: ..
mux:
  backend: zellij
  bars: true
  sessions:
    dev:
      windows:
        - {name: code, dir: api}
    ops:
      session: xtracta-ops
      windows:
        - {name: logs, dir: infra}
remotes:
  origin: git@example.com:org
repos:
  api:
  infra:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.Len(t, m.Mux.Sessions, 2)
	assert.Len(t, m.Mux.Sessions["dev"].Windows, 1)
	assert.Equal(t, "xtracta-ops", m.Mux.Sessions["ops"].Session)
}

func TestMuxConfig_ResolveSession_Named(t *testing.T) {
	cfg := MuxConfig{
		Sessions: map[string]MuxSession{
			"dev": {Windows: []MuxWindow{{Name: "code"}}},
			"ops": {Session: "custom-name", Windows: []MuxWindow{{Name: "logs"}}},
		},
	}
	s, name, err := cfg.ResolveSession("dev", "/ws")
	require.NoError(t, err)
	assert.Equal(t, "dev", name)
	assert.Len(t, s.Windows, 1)

	s, name, err = cfg.ResolveSession("ops", "/ws")
	require.NoError(t, err)
	assert.Equal(t, "custom-name", name)
}

func TestMuxConfig_ResolveSession_SingleDefault(t *testing.T) {
	cfg := MuxConfig{
		Sessions: map[string]MuxSession{
			"dev": {Windows: []MuxWindow{{Name: "code"}}},
		},
	}
	s, name, err := cfg.ResolveSession("", "/ws")
	require.NoError(t, err)
	assert.Equal(t, "dev", name)
	assert.Len(t, s.Windows, 1)
}

func TestMuxConfig_ResolveSession_MultipleRequiresName(t *testing.T) {
	cfg := MuxConfig{
		Sessions: map[string]MuxSession{
			"dev": {},
			"ops": {},
		},
	}
	_, _, err := cfg.ResolveSession("", "/ws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "specify one by name")
}

func TestMuxConfig_ResolveSession_Legacy(t *testing.T) {
	cfg := MuxConfig{
		Session: "my-session",
		Windows: []MuxWindow{{Name: "code"}},
	}
	s, name, err := cfg.ResolveSession("", "/ws")
	require.NoError(t, err)
	assert.Equal(t, "my-session", name)
	assert.Len(t, s.Windows, 1)
}

func TestMuxConfig_ResolveSession_LegacyFallbackToDir(t *testing.T) {
	cfg := MuxConfig{
		Windows: []MuxWindow{{Name: "code"}},
	}
	_, name, err := cfg.ResolveSession("", "/home/user/my-workspace")
	require.NoError(t, err)
	assert.Equal(t, "my-workspace", name)
}

func TestMuxConfig_ResolveSession_UnknownName(t *testing.T) {
	cfg := MuxConfig{
		Sessions: map[string]MuxSession{
			"dev": {},
		},
	}
	_, _, err := cfg.ResolveSession("nope", "/ws")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown mux session")
	assert.Contains(t, err.Error(), "dev")
}

func TestParse_MuxBarsDefaultFalse(t *testing.T) {
	yaml := `
root: ..
mux:
  backend: tmux
remotes:
  origin: git@example.com:org
repos:
  my-repo:
`
	m, err := Parse([]byte(yaml))
	require.NoError(t, err)
	assert.False(t, m.Mux.Bars)
}

func TestParse_RequiresRoot(t *testing.T) {
	_, err := Parse([]byte(`
remotes:
  origin: git@example.com:org
repos:
  my-repo:
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "root is required")
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
			yaml := fmt.Sprintf("root: ..\nremotes:\n  origin: git@example.com:org\nrepos:\n  %q:\n", name)
			_, err := Parse([]byte(yaml))
			assert.Error(t, err, "expected error for repo name %q", name)
		})
	}
}

func TestParse_RejectsCommaGroupName(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
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

	// Manifest root is explicitly "..", so path = wsHome/../repo-name
	path := m.ResolvePath("/home/user/workspace", "api-server", m.Repos["api-server"])
	assert.Equal(t, "/home/user/api-server", path)
}

func TestResolvePath_PerRepoRoot(t *testing.T) {
	yaml := `
root: ..
remotes:
  origin: git@example.com:org
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

func TestResolveRemotes_MergesTopLevelAndPerRepo(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	effective := m.ResolveRemotes("upstream-lib", m.Repos["upstream-lib"])
	assert.Equal(t, "git@github.com:acme-corp/upstream-lib.git", effective["origin"])
	assert.Equal(t, "git@github.com:open-source-org/upstream-lib.git", effective["upstream"])
}

func TestResolveRemotes_PerRepoOverridesTopLevel(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	effective := m.ResolveRemotes("custom-tool", m.Repos["custom-tool"])
	assert.Equal(t, "git@custom:org/repo.git", effective["origin"])
}

func TestRepoInfoFor_PopulatesRemotesAndDefaultCompare(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	info := m.RepoInfoFor(testWSHome, "upstream-lib", m.Repos["upstream-lib"], nil)
	assert.Equal(t, "upstream", info.DefaultCompare)
	assert.Len(t, info.Remotes, 2)
	assert.Equal(t, info.URL, info.Remotes["origin"])
}

func TestParse_RejectsLegacyURL(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
repos:
  legacy-repo: { url: git@github.com:org/repo.git }
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer supported")
	assert.Contains(t, err.Error(), `"url"`)
}

func TestParse_RejectsLegacyRemote(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
repos:
  legacy-repo: { remote: upstream }
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no longer supported")
	assert.Contains(t, err.Error(), `"remote"`)
}

func TestParse_RejectsMissingOrigin(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
repos:
  orphan:
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no effective origin")
}

func TestParse_RejectsUnknownDefaultCompare(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
repos:
  my-repo:
    default_compare: bogus
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "default_compare")
	assert.Contains(t, err.Error(), "bogus")
}

func TestParse_RejectsUnknownKey(t *testing.T) {
	_, err := Parse([]byte(`
root: ..
remotes:
  origin: git@example.com:org
repos:
  my-repo: { foo: bar }
`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `unknown key "foo"`)
}

func TestMergeLocal_RemotesMergeKeyByKey(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.yml"), []byte(`
root: ..
remotes:
  origin: git@github.com:acme
repos:
  lib:
    remotes:
      upstream: git@github.com:open/lib.git
    default_compare: upstream
`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "manifest.local.yml"), []byte(`
repos:
  lib:
    remotes:
      fork: git@github.com:me/lib.git
      upstream: git@github.com:new-owner/lib.git
`), 0644))

	m, err := LoadWithLocal(dir)
	require.NoError(t, err)

	cfg := m.Repos["lib"]
	// Added key from local
	assert.Equal(t, "git@github.com:me/lib.git", cfg.Remotes["fork"])
	// Overridden key
	assert.Equal(t, "git@github.com:new-owner/lib.git", cfg.Remotes["upstream"])
	// Preserved scalar from main
	assert.Equal(t, "upstream", cfg.DefaultCompare)
}

func repoNames(repos []RepoInfo) []string {
	names := make([]string, len(repos))
	for i, r := range repos {
		names[i] = r.Name
	}
	return names
}

func TestCloneToBrowseURL(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
		err  bool
	}{
		{"ssh-shorthand", "git@github.com:acme/foo.git", "https://github.com/acme/foo", false},
		{"ssh-shorthand-no-git", "git@bitbucket.org:xtracta/xtracta-compose", "https://bitbucket.org/xtracta/xtracta-compose", false},
		{"https-with-git", "https://bitbucket.org/xtracta/xtracta-compose.git", "https://bitbucket.org/xtracta/xtracta-compose", false},
		{"https-trailing-slash", "https://bitbucket.org/xtracta/xtracta-compose/", "https://bitbucket.org/xtracta/xtracta-compose", false},
		{"https-plain", "https://github.com/acme/foo", "https://github.com/acme/foo", false},
		{"ssh-scheme", "ssh://git@github.com/acme/foo.git", "https://github.com/acme/foo", false},
		{"git-scheme", "git://github.com/acme/foo.git", "https://github.com/acme/foo", false},
		{"http-scheme", "http://host.local/org/repo.git", "https://host.local/org/repo", false},
		{"ssh-with-port", "ssh://git@ssh.github.com:443/acme/foo.git", "https://ssh.github.com/acme/foo", false},
		{"empty", "", "", true},
		{"bad-shorthand-no-colon", "git@github.com/acme/foo.git", "", true},
		{"bad-scheme-no-path", "https://github.com", "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := CloneToBrowseURL(c.in)
			if c.err {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, c.want, got)
		})
	}
}

func TestBrowseURL(t *testing.T) {
	m, err := Parse([]byte(testManifest))
	require.NoError(t, err)

	// Declared repo uses the top-level prefix.
	got, err := m.BrowseURL("api-server")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme-corp/api-server", got)

	// Per-repo origin override wins over the top-level prefix.
	got, err = m.BrowseURL("custom-tool")
	require.NoError(t, err)
	assert.Equal(t, "https://custom/org/repo", got)

	// A name not in the manifest falls back to the top-level origin prefix.
	got, err = m.BrowseURL("not-declared")
	require.NoError(t, err)
	assert.Equal(t, "https://github.com/acme-corp/not-declared", got)

	// Invalid name is rejected before URL resolution.
	_, err = m.BrowseURL("foo/bar")
	assert.Error(t, err)
	_, err = m.BrowseURL(".")
	assert.Error(t, err)

	// No top-level origin and not declared -> error.
	empty := &Manifest{Repos: map[string]RepoConfig{}, Remotes: map[string]string{}}
	_, err = empty.BrowseURL("anything")
	assert.Error(t, err)
}
