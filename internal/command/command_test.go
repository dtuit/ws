package command

import (
	"encoding/json"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFocusJSON_PreservesSettings(t *testing.T) {
	input := `{
  "folders": [
    {"name": "~ workspace", "path": "."},
    {"name": "old-repo", "path": "../old-repo"}
  ],
  "settings": {
    "files.exclude": {"**/.git": true}
  }
}`
	repos := []manifest.RepoInfo{
		{Name: "repo-a"},
		{Name: "repo-b"},
	}

	out, err := FocusJSON([]byte(input), repos, "..")
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))

	// Settings preserved
	settings, ok := ws["settings"].(map[string]interface{})
	require.True(t, ok)
	assert.Contains(t, settings, "files.exclude")

	// Folders updated
	folders, ok := ws["folders"].([]interface{})
	require.True(t, ok)
	assert.Len(t, folders, 3) // workspace + 2 repos

	// First folder is workspace
	first := folders[0].(map[string]interface{})
	assert.Equal(t, "~ workspace", first["name"])

	// Second folder is repo-a
	second := folders[1].(map[string]interface{})
	assert.Equal(t, "repo-a", second["name"])
	assert.Equal(t, "../repo-a", second["path"])
}

func TestFocusJSON_EmptyRepos(t *testing.T) {
	input := `{"folders": [{"name": "~ workspace", "path": "."}], "settings": {}}`

	out, err := FocusJSON([]byte(input), nil, "..")
	require.NoError(t, err)

	var ws map[string]interface{}
	require.NoError(t, json.Unmarshal(out, &ws))
	folders := ws["folders"].([]interface{})
	assert.Len(t, folders, 1) // just workspace
}

func TestParseSuperArgs_WithGroup(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
groups:
  ai: [repo-a]
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs := ParseSuperArgs(m, []string{"ai", "git", "status"})
	assert.Equal(t, "ai", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
}

func TestParseSuperArgs_WithoutGroup(t *testing.T) {
	m, err := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))
	require.NoError(t, err)

	filter, cmdArgs := ParseSuperArgs(m, []string{"git", "status"})
	assert.Equal(t, "", filter)
	assert.Equal(t, []string{"git", "status"}, cmdArgs)
}

func TestParseSuperArgs_Empty(t *testing.T) {
	m, _ := manifest.Parse([]byte(`
remotes:
  default: git@example.com
repos:
  repo-a:
`))

	filter, cmdArgs := ParseSuperArgs(m, nil)
	assert.Equal(t, "", filter)
	assert.Nil(t, cmdArgs)
}
