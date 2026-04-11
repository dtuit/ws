package command

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStripLLBranchesFlags(t *testing.T) {
	args, showBranches := StripLLBranchesFlags([]string{"-t", "--branches", "backend"})
	assert.Equal(t, []string{"-t", "backend"}, args)
	assert.True(t, showBranches)

	args, showBranches = StripLLBranchesFlags([]string{"repo-a"})
	assert.Equal(t, []string{"repo-a"}, args)
	assert.False(t, showBranches)
}

func TestLLWithBranches_PrintsCurrentAndOtherLocalBranches(t *testing.T) {
	wsHome := t.TempDir()
	repoPath := filepath.Join(wsHome, "repos", "api-server")

	initCheckout(t, repoPath)
	runGit(t, repoPath, "branch", "feature/one")
	runGit(t, repoPath, "checkout", "-b", "feature/auth")
	require.NoError(t, os.WriteFile(filepath.Join(repoPath, "scratch.txt"), []byte("dirty\n"), 0644))

	m, err := parseManifestYAML(`
remotes:
  default: git@example.com
branch: master
repos:
  api-server:
`)
	require.NoError(t, err)

	output := captureStdout(t, func() {
		require.NoError(t, LL(m, wsHome, "", false, LLMode{ShowBranches: true}))
	})

	assert.Contains(t, output, "api-server")
	assert.Contains(t, output, "feature/auth")
	assert.Equal(t, 1, strings.Count(output, "feature/auth"))
	assert.Contains(t, output, "feature/one")
	assert.Contains(t, output, "master")
	assert.Contains(t, output, "[?   ]")
	assert.NotContains(t, output, "[    ]")
}

func captureStdout(t *testing.T, fn func()) string {
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
