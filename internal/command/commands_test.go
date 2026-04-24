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
		CommandBrowse,
		CommandRepos,
		CommandDirs,
		CommandFetch,
		CommandPull,
		CommandContext,
		CommandMux,
		CommandWorktree,
		CommandRemotes,
		CommandAgent,
	}, BuiltinCommandNames())
}

func TestBuiltinCommandSuggestions(t *testing.T) {
	assert.Contains(t, BuiltinCommandSuggestions(), CommandContext)
	assert.Contains(t, BuiltinCommandSuggestions(), "ctx")
}

func TestBuiltinUsageEntries(t *testing.T) {
	entries := BuiltinUsageEntries()

	assert.Contains(t, entries, HelpEntry{
		Usage:       "context set <filter>",
		Description: "Explicit form of context set",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "context refresh [" + WorktreesFlagUsage + "]",
		Description: "Re-resolve the stored context and rebuild scope",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "ll [filter]",
		Description: "Dashboard: branch, dirty, last commit",
	})
	assert.Contains(t, entries, HelpEntry{
		Usage:       "ll [--branches|-b] [filter]",
		Description: "Show all local branches in ll format",
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

func TestUsageTextGroupsByCategory(t *testing.T) {
	out := UsageText()

	// Category headings appear in order.
	for _, heading := range []string{"Inspect:", "Sync:", "Scope:", "Tools:", "Install:"} {
		assert.Contains(t, out, heading)
	}

	// Aliases render parenthetically in the description.
	assert.Contains(t, out, "(alias: ctx)")
	assert.Contains(t, out, "(alias: wt)")
	assert.Contains(t, out, "(alias: list)")

	// Dedicated alias rows are gone.
	assert.NotContains(t, out, "ctx [filter]    Alias")

	// Filters block and examples are present.
	assert.Contains(t, out, "Filters (apply to most commands):")
	assert.Contains(t, out, "Examples:")
	assert.Contains(t, out, "ws pull backend")
}

func TestResolveBuiltinCommandName(t *testing.T) {
	assert.Equal(t, CommandContext, ResolveBuiltinCommandName("ctx"))
	assert.Equal(t, CommandLL, ResolveBuiltinCommandName(CommandLL))
}
