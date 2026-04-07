package command

const (
	CommandHelp    = "help"
	CommandVersion = "version"
	CommandLL      = "ll"
	CommandCD      = "cd"
	CommandSetup   = "setup"
	CommandShell   = "shell"
	CommandOpen    = "open"
	CommandList    = "list"
	CommandFetch   = "fetch"
	CommandPull    = "pull"
	CommandContext = "context"
)

// HelpEntry is a single usage line plus its description.
type HelpEntry struct {
	Usage       string
	Description string
}

// BuiltinCommand describes one top-level built-in command.
type BuiltinCommand struct {
	Name        string
	ShowInUsage bool
	Help        []HelpEntry
}

var builtinCommands = []BuiltinCommand{
	{
		Name: CommandHelp,
	},
	{
		Name: CommandVersion,
	},
	{
		Name:        CommandLL,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "ll [filter] [-t|--worktrees|--no-worktrees]", Description: "Dashboard: branch, dirty, last commit"},
		},
	},
	{
		Name:        CommandCD,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "cd [repo[@worktree]] [--worktree|-t <selector>]", Description: "Print repo path (no arg = workspace root)"},
		},
	},
	{
		Name:        CommandSetup,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "setup [filter]", Description: "Clone missing repos"},
		},
	},
	{
		Name:        CommandShell,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "shell init", Description: "Emit shell integration and completion"},
			{Usage: "shell install", Description: "Write shell config for ws cd and completion"},
		},
	},
	{
		Name:        CommandOpen,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: CommandOpen, Description: "Open the current VS Code workspace"},
		},
	},
	{
		Name:        CommandList,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "list [--all] [-t|--worktrees|--no-worktrees]", Description: "Show repos in manifest (--all includes excluded)"},
		},
	},
	{
		Name:        CommandFetch,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "fetch [filter]", Description: "Fetch all repos"},
		},
	},
	{
		Name:        CommandPull,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "pull [filter] [-t|--worktrees|--no-worktrees]", Description: "Pull manifest checkouts or all discovered worktrees"},
		},
	},
	{
		Name:        CommandContext,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "context [-t|--worktrees|--no-worktrees] [filter]", Description: `Set default filter (no arg = show, "none" = clear)`},
			{Usage: "context set [-t|--worktrees|--no-worktrees] <filter>", Description: "Explicit form of context set"},
			{Usage: "context add [-t|--worktrees|--no-worktrees] <filter>", Description: "Add groups or repos to the existing context"},
			{Usage: "context remove [-t|--worktrees|--no-worktrees] <filter>", Description: "Remove groups or repos from the existing context"},
			{Usage: "context save [--local] <group>", Description: "Persist the current context as a named group"},
		},
	},
}

// BuiltinCommands returns the registered top-level command metadata.
func BuiltinCommands() []BuiltinCommand {
	out := make([]BuiltinCommand, 0, len(builtinCommands))
	for _, cmd := range builtinCommands {
		help := append([]HelpEntry(nil), cmd.Help...)
		out = append(out, BuiltinCommand{
			Name:        cmd.Name,
			ShowInUsage: cmd.ShowInUsage,
			Help:        help,
		})
	}
	return out
}

// BuiltinCommandNames returns the names of all top-level built-in commands.
func BuiltinCommandNames() []string {
	names := make([]string, 0, len(builtinCommands))
	for _, cmd := range builtinCommands {
		names = append(names, cmd.Name)
	}
	return names
}

// BuiltinUsageEntries returns the help entries shown in `ws help`.
func BuiltinUsageEntries() []HelpEntry {
	var entries []HelpEntry
	for _, cmd := range builtinCommands {
		if !cmd.ShowInUsage {
			continue
		}
		entries = append(entries, cmd.Help...)
	}
	return entries
}
