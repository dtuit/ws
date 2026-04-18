package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestUsageTextIncludesShellCommands(t *testing.T) {
	text := UsageText()

	assert.Contains(t, text, "shell init|install")
	assert.Contains(t, text, "-t, --worktrees")
	assert.Contains(t, text, "--no-worktrees")
	assert.Contains(t, text, "(alias: ctx)")
	assert.Contains(t, text, "active[:dur]")
	assert.Contains(t, text, "dirty")
	assert.Contains(t, text, "mine:<dur>")
	assert.Contains(t, text, "ws <command> --help")
	assert.NotContains(t, text, "%s")
	assert.NotContains(t, text, "\n  help ")
	assert.NotContains(t, text, "\n  version ")
}
