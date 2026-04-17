package command

import (
	"fmt"
	"strings"
)

const commandHelpSummaryIndent = 25

// UsageText renders the `ws help` output from the shared command metadata.
func UsageText() string {
	var b strings.Builder
	b.WriteString("Usage: ws [-w <path>] <command> [args]\n\nCommands:\n")
	for _, entry := range BuiltinUsageEntries() {
		writeUsageEntry(&b, entry)
	}
	b.WriteString(`
Worktree options:
  -t, --worktrees     Expand repo/group filters to linked worktrees
  --no-worktrees      Force primary checkouts only

Context shorthand:
  ctx                 Alias for "context"

Any unrecognized command is run across repos:
  ws git status          Run "git status" in all repos
  ws -t git status
                         Run "git status" in all discovered worktrees
  ws ai git log -1       Run "git log -1" in a group
  ws ls -la              Any command, not just git

Use -- to escape built-in names:
  ws -- fetch data.json
                         Run "fetch data.json" (not git fetch)

Filters:
  all                    All active repos (default)
  dirty                  Repos with uncommitted changes
  active[:dur]           dirty or local-user commits within dur
  mine:<dur>             local-user commits within dur
  dur                    Positive duration with s, m, h, d, or w suffix
  <group>                Group name: ai, eng, db, inf
  <group>,<group>        Comma-separated groups
  <repo>                 Individual repo name
`)
	return b.String()
}

func writeUsageEntry(b *strings.Builder, entry HelpEntry) {
	if len(entry.Usage) <= commandHelpSummaryIndent-2 {
		fmt.Fprintf(b, "  %-*s %s\n", commandHelpSummaryIndent-2, entry.Usage, entry.Description)
		return
	}
	fmt.Fprintf(b, "  %s\n%*s%s\n", entry.Usage, commandHelpSummaryIndent, "", entry.Description)
}

// CommandHelpText returns the detailed help text for a named command.
// If the command has a DetailedHelp string, that is returned. Otherwise
// the help entries are formatted into a short summary.
func CommandHelpText(name string) (string, bool) {
	cmd, ok := builtinCommandByName(name)
	if !ok {
		return "", false
	}

	if cmd.DetailedHelp != "" {
		return cmd.DetailedHelp, true
	}

	if len(cmd.Help) == 0 {
		return "", false
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Usage:\n")
	for _, entry := range cmd.Help {
		fmt.Fprintf(&b, "  ws %s\n", entry.Usage)
		fmt.Fprintf(&b, "      %s\n\n", entry.Description)
	}
	return b.String(), true
}
