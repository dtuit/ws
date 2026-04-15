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
	CommandMux      = "mux"
	CommandWorktree = "worktree"
)

type builtinCommandAlias struct {
	Alias string
	Name  string
}

var builtinCommandAliases = []builtinCommandAlias{
	{Alias: "ctx", Name: CommandContext},
	{Alias: "wt", Name: CommandWorktree},
}

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
	complete    CompletionHandler
}

var builtinCommands = []BuiltinCommand{
	{
		Name:     CommandHelp,
		complete: completeNoopCommand,
	},
	{
		Name:     CommandVersion,
		complete: completeNoopCommand,
	},
	{
		Name:        CommandLL,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "ll [filter]", Description: "Dashboard: branch, dirty, last commit"},
			{Usage: "ll [" + LLBranchesFlagUsage + "] [filter]", Description: "Show all local branches in ll format"},
		},
		complete: completeLLCommand,
	},
	{
		Name:        CommandCD,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "cd [repo[@worktree]] [" + CDWorktreeFlagUsage + " <selector>]", Description: "Print repo path (no arg = workspace root)"},
		},
		complete: completeCDCommand,
	},
	{
		Name:        CommandSetup,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "setup [filter]", Description: "Clone missing repos"},
		},
		complete: completeSetupCommand,
	},
	{
		Name:        CommandShell,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "shell init", Description: "Emit shell integration and completion"},
			{Usage: "shell install", Description: "Write shell config for ws cd and completion"},
		},
		complete: completeShellCommand,
	},
	{
		Name:        CommandOpen,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "open [--editor <name>]", Description: "Open workspace (default: code, or WS_EDITOR)"},
		},
		complete: completeNoopCommand,
	},
	{
		Name:        CommandList,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "list [--all]", Description: "Show repos in manifest (--all includes excluded)"},
		},
		complete: completeListCommand,
	},
	{
		Name:        CommandFetch,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "fetch [filter]", Description: "Fetch all repos"},
		},
		complete: completeFetchCommand,
	},
	{
		Name:        CommandPull,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "pull [filter]", Description: "Pull repos in scope"},
		},
		complete: completeLLOrPullCommand,
	},
	{
		Name:        CommandContext,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "context [filter]", Description: "Set default filter (no arg = show, " + ContextClearUsage + " = clear)"},
			{Usage: "ctx [filter]", Description: "Alias for context"},
			{Usage: "context set <filter>", Description: "Explicit form of context set"},
			{Usage: "context refresh [" + WorktreesFlagUsage + "]", Description: "Re-resolve the stored context and rebuild scope"},
			{Usage: "context . [" + WorktreesFlagUsage + "]", Description: "Shorthand for context refresh"},
			{Usage: "context -", Description: "Swap to the previous context (like cd -)"},
			{Usage: "context add <filter>", Description: "Add groups or repos to the existing context"},
			{Usage: "context remove <filter>", Description: "Remove groups or repos from the existing context"},
			{Usage: "context save [--local] <group>", Description: "Persist the current context as a named group"},
		},
		complete: completeContextCommand,
	},
	{
		Name:        CommandMux,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "mux [session]", Description: "Attach or create a terminal session (tmux/zellij)"},
			{Usage: "mux kill [session]", Description: "Kill a session"},
			{Usage: "mux ls", Description: "List multiplexer sessions"},
			{Usage: "mux dup [window]", Description: "Duplicate a window/tab in the active session"},
			{Usage: "mux save [--local] [session]", Description: "Save session layout to manifest"},
		},
		complete: completeMuxCommand,
	},
	{
		Name:        CommandWorktree,
		ShowInUsage: true,
		Help: []HelpEntry{
			{Usage: "worktree add <branch> [filter]", Description: "Create worktrees across repos"},
			{Usage: "worktree remove <branch> [filter]", Description: "Remove worktrees across repos"},
			{Usage: "worktree list [filter]", Description: "List worktrees per repo"},
			{Usage: "wt add <branch> [filter]", Description: "Alias for worktree"},
		},
		complete: completeWorktreeCommand,
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

// BuiltinCommandSuggestions returns canonical command names plus shorthand aliases.
func BuiltinCommandSuggestions() []string {
	names := BuiltinCommandNames()
	for _, alias := range builtinCommandAliases {
		names = append(names, alias.Alias)
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

// ResolveBuiltinCommandName maps a shorthand alias to its canonical built-in name.
func ResolveBuiltinCommandName(name string) string {
	for _, alias := range builtinCommandAliases {
		if alias.Alias == name {
			return alias.Name
		}
	}
	return name
}

func builtinCommandByName(name string) (BuiltinCommand, bool) {
	name = ResolveBuiltinCommandName(name)
	for _, cmd := range builtinCommands {
		if cmd.Name == name {
			return cmd, true
		}
	}
	return BuiltinCommand{}, false
}
