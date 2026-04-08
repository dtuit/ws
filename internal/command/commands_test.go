package command

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestBuiltinCommandNames(t *testing.T) {
	assert.Equal(t, []string{
		CommandHelp,
		CommandVersion,
		CommandLL,
		CommandCD,
		CommandSetup,
		CommandShell,
		CommandOpen,
		CommandList,
		CommandFetch,
		CommandPull,
		CommandContext,
	}, BuiltinCommandNames())
}

func TestBuiltinUsageEntries(t *testing.T) {
	entries := BuiltinUsageEntries()

	assert.Contains(t, entries, HelpEntry{
		Usage:       "context set <filter>",
		Description: "Explicit form of context set",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "ll [filter]",
		Description: "Dashboard: branch, dirty, last commit",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "cd [repo[@worktree]] [--worktree|-t <selector>]",
		Description: "Print repo path (no arg = workspace root)",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "shell install",
		Description: "Write shell config for ws cd and completion",
	})
	assert.NotContains(t, entries, HelpEntry{
		Usage:       CommandHelp,
		Description: "",
	})
	assert.NotContains(t, entries, HelpEntry{
		Usage:       CommandVersion,
		Description: "",
	})
}
