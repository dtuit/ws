package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageTextIncludesShellCommands(t *testing.T) {
	text := UsageText()

	assert.Contains(t, text, "shell init")
	assert.Contains(t, text, "shell install")
	assert.Contains(t, text, "Worktree options:")
	assert.Contains(t, text, "-t, --worktrees")
	assert.Contains(t, text, "--no-worktrees")
	assert.Contains(t, text, "Context shorthand:")
	assert.Contains(t, text, "ctx")
	assert.NotContains(t, text, "%s")
	assert.NotContains(t, text, "\n  help ")
	assert.NotContains(t, text, "\n  version ")
}
