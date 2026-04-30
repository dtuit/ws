package command

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func runGitEnv(t *testing.T, dir string, env []string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	output, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v failed: %s", args, string(output))
}

func parseManifestYAML(yaml string) (*manifest.Manifest, error) {
	yaml = injectTestDefaults(yaml)
	return manifest.Parse([]byte(yaml))
}

// injectTestDefaults ensures test YAML fragments parse under the post-multi-remote
// validation: adds `root:` and a `remotes.origin` prefix if missing.
func injectTestDefaults(yaml string) string {
	if !strings.Contains(yaml, "\nroot:") && !strings.HasPrefix(strings.TrimSpace(yaml), "root:") {
		yaml = "root: repos\n" + yaml
	}
	if !strings.Contains(yaml, "\nremotes:") && !strings.HasPrefix(strings.TrimSpace(yaml), "remotes:") {
		yaml = "remotes:\n  origin: git@test:org\n" + yaml
	}
	return yaml
}

func readStoredContext(t *testing.T, wsHome string) contextState {
	t.Helper()

	data, err := os.ReadFile(stateReadPath(wsHome, contextStateFile, legacyContextFile))
	require.NoError(t, err)

	var state contextState
	require.NoError(t, yaml.Unmarshal(data, &state))
	return state
}

func loadManifestWithLocal(t *testing.T, wsHome, manifestYAML, localYAML string) *manifest.Manifest {
	t.Helper()

	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.yml"), []byte(injectTestDefaults(manifestYAML)), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(wsHome, "manifest.local.yml"), []byte(localYAML), 0644))

	m, err := manifest.LoadWithLocal(wsHome)
	require.NoError(t, err)
	return m
}

func initCheckout(t *testing.T, repoPath string) {
	t.Helper()

	require.NoError(t, os.MkdirAll(filepath.Dir(repoPath), 0755))
	runGit(t, filepath.Dir(repoPath), "init", filepath.Base(repoPath))
	runGit(t, repoPath, "config", "user.name", "Test User")
	runGit(t, repoPath, "config", "user.email", "test@example.com")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "README.md"), []byte("hello\n"), 0644))
	runGit(t, repoPath, "add", "README.md")
	runGit(t, repoPath, "commit", "-m", "init")
}

func commitEmptyAt(t *testing.T, repoPath, message, name, email string, when time.Time) {
	t.Helper()

	timestamp := when.UTC().Format(time.RFC3339)
	runGitEnv(t, repoPath, []string{
		"GIT_AUTHOR_NAME=" + name,
		"GIT_AUTHOR_EMAIL=" + email,
		"GIT_COMMITTER_NAME=" + name,
		"GIT_COMMITTER_EMAIL=" + email,
		"GIT_AUTHOR_DATE=" + timestamp,
		"GIT_COMMITTER_DATE=" + timestamp,
	}, "commit", "--allow-empty", "-m", message)
}

func assertScopeEntries(t *testing.T, wsHome string, want ...string) {
	t.Helper()

	assertScopeEntriesForWorkspace(t, wsHome, "", want...)
}

func assertScopeEntriesInDir(t *testing.T, wsHome, dir string, want ...string) {
	t.Helper()

	if dir != manifest.DefaultScopeDir {
		t.Fatalf("scope dir %s is no longer supported; use workspace scope hints", dir)
	}
	assertScopeEntriesForWorkspace(t, wsHome, "", want...)
}

func assertScopeEntriesForWorkspace(t *testing.T, wsHome, workspace string, want ...string) {
	t.Helper()

	data, err := os.ReadFile(workspaceScopeHintPath(wsHome, workspace))
	require.NoError(t, err)

	var hint scopeHint
	require.NoError(t, yaml.Unmarshal(data, &hint))

	var got []string
	for _, repo := range hint.Repos {
		got = append(got, repo.Name)
	}
	assert.Equal(t, want, got)
}

func assertNoScopeDir(t *testing.T, wsHome, dir string) {
	t.Helper()

	if dir != manifest.DefaultScopeDir {
		t.Fatalf("scope dir %s is no longer supported; use workspace scope hints", dir)
	}

	_, err := os.Stat(workspaceScopeHintPath(wsHome, ""))
	assert.True(t, os.IsNotExist(err), "expected %s to be absent", workspaceScopeHintPath(wsHome, ""))
}

func assertWorkspaceFolders(t *testing.T, workspacePath string, want ...string) {
	t.Helper()

	data, err := os.ReadFile(workspacePath)
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &ws))

	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)

	var got []string
	for _, raw := range folders {
		folder, ok := raw.(map[string]interface{})
		require.True(t, ok)
		name, ok := folder["name"].(string)
		require.True(t, ok)
		got = append(got, name)
	}

	assert.Equal(t, want, got)
}

func repoNames(repos []manifest.RepoInfo) []string {
	names := make([]string, len(repos))
	for i, repo := range repos {
		names[i] = repo.Name
	}
	return names
}

func captureCommandStdout(t *testing.T, fn func()) string {
	t.Helper()

	original := os.Stdout
	reader, writer, err := os.Pipe()
	require.NoError(t, err)
	os.Stdout = writer

	defer func() {
		os.Stdout = original
	}()

	fn()

	require.NoError(t, writer.Close())
	data, err := io.ReadAll(reader)
	require.NoError(t, err)
	require.NoError(t, reader.Close())
	return string(data)
}
