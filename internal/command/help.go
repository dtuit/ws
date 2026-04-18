package command

import (
	"fmt"
	"strings"
)

// Description column for `ws help`. Wide enough for "context|ctx [subcommand]".
const commandHelpSummaryIndent = 27

// UsageText renders the `ws help` output grouped by category.
func UsageText() string {
	var b strings.Builder
	b.WriteString("Usage: ws [-w <path>] <command> [args]\n")

	for _, cat := range categoryOrder {
		cmds := commandsInCategory(cat)
		if len(cmds) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s:\n", cat)
		for _, cmd := range cmds {
			writeUsageEntry(&b, cmd.summaryLine())
		}
	}

	b.WriteString(`
Filters (apply to most commands):
  all                    All active repos (default)
  dirty                  Repos with uncommitted changes
  active[:dur]           Dirty or your commits within dur (default 14d)
  mine:<dur>             Your commits within dur
  <group>                Named group from manifest
  <repo>                 Individual repo name
  <repo>,<group>         Comma-separated mix of repos and groups
  dur suffixes           s, m, h, d, w

Worktrees:
  -t, --worktrees        Expand filters to include linked worktrees
  --no-worktrees         Primary checkouts only

Examples:
  ws ll dirty            Status of repos with uncommitted changes
  ws pull backend        Pull the "backend" repo group (from manifest)
  ws ll active:7d        Repos active in the last week
  ws dirs mine:1w        Paths for repos you touched this week
  ws -t git status       Run any command across repos + worktrees
  ws -- fetch data.json  Escape built-in names (runs plain "fetch")

See ` + "`ws <command> --help`" + ` for options, subcommands, and examples.
`)
	return b.String()
}

// summaryLine returns the HelpEntry shown for this command in `ws help`.
// Aliases are noted parenthetically in the description.
func (c BuiltinCommand) summaryLine() HelpEntry {
	summary := c.Summary
	if summary.Usage == "" && summary.Description == "" && len(c.Help) > 0 {
		// Fall back to the first Help entry, stripping the name prefix.
		first := c.Help[0]
		summary.Description = first.Description
		summary.Usage = strings.TrimLeft(strings.TrimPrefix(first.Usage, c.Name), " ")
	}

	usage := c.Name
	if summary.Usage != "" {
		usage += " " + summary.Usage
	}

	description := summary.Description
	if len(c.Aliases) > 0 {
		description += fmt.Sprintf(" (alias: %s)", strings.Join(c.Aliases, ", "))
	}
	return HelpEntry{Usage: usage, Description: description}
}

func commandsInCategory(cat Category) []BuiltinCommand {
	var out []BuiltinCommand
	for _, cmd := range builtinCommands {
		if !cmd.ShowInUsage || cmd.Category != cat {
			continue
		}
		out = append(out, cmd)
	}
	return out
}

func writeUsageEntry(b *strings.Builder, entry HelpEntry) {
	const prefix = "  "
	usageBudget := commandHelpSummaryIndent - len(prefix) - 1
	if len(entry.Usage) <= usageBudget {
		fmt.Fprintf(b, "%s%-*s %s\n", prefix, usageBudget, entry.Usage, entry.Description)
		return
	}
	fmt.Fprintf(b, "%s%s\n%*s%s\n", prefix, entry.Usage, commandHelpSummaryIndent, "", entry.Description)
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
