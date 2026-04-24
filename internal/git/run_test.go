package git

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/dtuit/ws/internal/manifest"
	"github.com/dtuit/ws/internal/term"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFormatRunPrefix_UsesRepoColorAndDimSeparator(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	got := formatRunPrefix("repo", 6, 1, true)

	assert.Equal(t, term.Colorize(runPrefixPalette[1], "repo  ")+term.Colorize(term.Dim, " | "), got)
}

func TestExec_ColorsPassThroughPrefixes(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	dir := t.TempDir()
	repoA := initTestRepo(t, dir, "alpha")
	repoB := initTestRepo(t, dir, "beta")

	repos := []manifest.RepoInfo{
		{Name: "alpha", Path: filepath.Clean(repoA)},
		{Name: "beta", Path: filepath.Clean(repoB)},
	}

	stdout, stderr := captureOutput(t, func() {
		failCount := Exec(repos, []string{"sh", "-c", "printf 'hello\\n'"}, 2)
		assert.Zero(t, failCount)
	})

	assert.Contains(t, stdout, formatRunPrefix("alpha", 5, 0, true)+"hello")
	assert.Contains(t, stdout, formatRunPrefix("beta", 5, 1, true)+"hello")
	assert.Empty(t, stderr)
}

func TestExec_GitOutputCarriesAnsiWhenEnabled(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "alpha")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("x"), 0644))

	repos := []manifest.RepoInfo{{Name: "alpha", Path: filepath.Clean(repoDir)}}

	stdout, _ := captureOutput(t, func() {
		failCount := Exec(repos, []string{"git", "status"}, 1)
		assert.Zero(t, failCount)
	})

	assert.Contains(t, stdout, "untracked.txt")
	assert.Contains(t, stdout, "\x1b[31m", "expected red ANSI code from git for untracked file")
}

func TestExec_GitOutputHasNoAnsiWhenDisabled(t *testing.T) {
	term.SetEnabled(false)

	dir := t.TempDir()
	repoDir := initTestRepo(t, dir, "alpha")
	require.NoError(t, os.WriteFile(filepath.Join(repoDir, "untracked.txt"), []byte("x"), 0644))

	repos := []manifest.RepoInfo{{Name: "alpha", Path: filepath.Clean(repoDir)}}

	stdout, _ := captureOutput(t, func() {
		failCount := Exec(repos, []string{"git", "status"}, 1)
		assert.Zero(t, failCount)
	})

	assert.Contains(t, stdout, "untracked.txt")
	assert.NotContains(t, stdout, "\x1b[", "expected no ANSI codes when color disabled")
}

func TestRunAll_LeavesPrefixesPlainUnlessEnabled(t *testing.T) {
	term.SetEnabled(true)
	defer term.SetEnabled(false)

	dir := t.TempDir()
	repo := initTestRepo(t, dir, "alpha")

	stdout, stderr := captureOutput(t, func() {
		failCount := RunAll([]manifest.RepoInfo{{Name: "alpha", Path: filepath.Clean(repo)}}, []string{"sh", "-c", "printf 'hello\\n'"}, 1, RunOpts{})
		assert.Zero(t, failCount)
	})

	assert.Contains(t, stdout, "alpha | hello")
	assert.NotContains(t, stdout, "\x1b[")
	assert.Empty(t, stderr)
}

func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()

	oldStdout := os.Stdout
	oldStderr := os.Stderr

	stdoutR, stdoutW, err := os.Pipe()
	require.NoError(t, err)
	stderrR, stderrW, err := os.Pipe()
	require.NoError(t, err)

	os.Stdout = stdoutW
	os.Stderr = stderrW

	stdoutCh := make(chan string, 1)
	stderrCh := make(chan string, 1)

	go func() {
		data, _ := io.ReadAll(stdoutR)
		stdoutCh <- string(data)
	}()
	go func() {
		data, _ := io.ReadAll(stderrR)
		stderrCh <- string(data)
	}()

	fn()

	require.NoError(t, stdoutW.Close())
	require.NoError(t, stderrW.Close())
	os.Stdout = oldStdout
	os.Stderr = oldStderr

	return <-stdoutCh, <-stderrCh
}
